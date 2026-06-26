package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/connector"
)

var binaryExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true,
	".svg": true, ".webp": true, ".bmp": true, ".tiff": true,
	".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
	".zip": true, ".tar": true, ".gz": true, ".rar": true, ".7z": true,
	".bin": true, ".exe": true, ".dll": true, ".so": true, ".dylib": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true,
	".mp3": true, ".mp4": true, ".wav": true, ".avi": true, ".mov": true,
	".pyc": true, ".class": true,
}

const maxFileBytes = 500 * 1024

var httpClient = &http.Client{Timeout: 30 * time.Second}

type Connector struct{}

func New() *Connector { return &Connector{} }

// FetchStream implements connector.StreamProvider.
// Checkpoints by file index so a pod restart can skip already-processed files.
func (c *Connector) FetchStream(ctx context.Context, cfg map[string]any, opts connector.FetchOpts, emit func(connector.Document) error) error {
	token, _ := cfg["token"].(string)
	owner, _ := cfg["owner"].(string)
	repo, _ := cfg["repo"].(string)
	branch, _ := cfg["branch"].(string)

	if token == "" || owner == "" || repo == "" {
		return fmt.Errorf("github: token, owner, and repo are required")
	}

	log := slog.With("connector", "github", "owner", owner, "repo", repo)

	do := func(u string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		return httpClient.Do(req)
	}

	if branch == "" {
		res, err := do(fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo))
		if err != nil {
			return fmt.Errorf("github: fetch repo info: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("github: repo info returned HTTP %d", res.StatusCode)
		}
		var info struct{ DefaultBranch string `json:"default_branch"` }
		if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
			return fmt.Errorf("github: decode repo info: %w", err)
		}
		branch = info.DefaultBranch
	}

	// Fetch the full recursive file tree (single API call).
	treeURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, branch)
	res, err := do(treeURL)
	if err != nil {
		return fmt.Errorf("github: fetch tree: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("github: tree returned HTTP %d", res.StatusCode)
	}
	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int    `json:"size"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(res.Body).Decode(&tree); err != nil {
		return fmt.Errorf("github: decode tree: %w", err)
	}

	// Filter to text blobs only.
	type blob struct{ path string; size int }
	var blobs []blob
	for _, item := range tree.Tree {
		if item.Type != "blob" {
			continue
		}
		if binaryExts[strings.ToLower(filepath.Ext(item.Path))] {
			continue
		}
		if item.Size > maxFileBytes {
			continue
		}
		blobs = append(blobs, blob{item.Path, item.Size})
	}
	log.Info("tree fetched", "total_blobs", len(blobs))

	// Load resume offset from checkpoint.
	var cp struct {
		ProcessedCount int `json:"processed_count"`
	}
	if len(opts.Checkpoint) > 2 {
		_ = json.Unmarshal(opts.Checkpoint, &cp)
	}
	if cp.ProcessedCount > 0 {
		log.Info("resuming from checkpoint", "skip_files", cp.ProcessedCount)
	}

	processed := 0
	for i, b := range blobs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if i < cp.ProcessedCount {
			continue
		}

		contentURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, b.path, branch)
		cres, err := do(contentURL)
		if err != nil {
			log.Warn("fetch file failed", "path", b.path, "error", err)
			continue
		}
		if cres.StatusCode != http.StatusOK {
			cres.Body.Close()
			log.Warn("fetch file non-200", "path", b.path, "status", cres.StatusCode)
			continue
		}
		var fileContent struct {
			Content  string `json:"content"`
			Encoding string `json:"encoding"`
		}
		if json.NewDecoder(cres.Body).Decode(&fileContent) != nil {
			cres.Body.Close()
			continue
		}
		cres.Body.Close()

		var text string
		if fileContent.Encoding == "base64" {
			decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(fileContent.Content, "\n", ""))
			if err != nil {
				continue
			}
			text = string(decoded)
		} else {
			text = fileContent.Content
		}

		if err := emit(connector.Document{
			Source:           "github",
			SourceDocumentID: b.path,
			Title:            filepath.Base(b.path),
			URL:              fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", owner, repo, branch, b.path),
			Author:           owner,
			Content:          text,
		}); err != nil {
			return err
		}

		processed++
		// Checkpoint every 10 files.
		if opts.SaveCheckpoint != nil && processed%10 == 0 {
			opts.SaveCheckpoint(map[string]any{"processed_count": i + 1})
		}
	}

	log.Info("github sync complete", "files_processed", processed)
	return nil
}

// Fetch implements connector.Provider via FetchStream.
func (c *Connector) Fetch(ctx context.Context, cfg map[string]any) ([]connector.Document, error) {
	var docs []connector.Document
	err := c.FetchStream(ctx, cfg, connector.FetchOpts{}, func(doc connector.Document) error {
		docs = append(docs, doc)
		return nil
	})
	return docs, err
}
