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

// multiCheckpoint is the resume state for multi-repo mode.
type multiCheckpoint struct {
	CompletedRepos []string `json:"completed_repos"`
	CurrentRepo    string   `json:"current_repo"`
	ProcessedCount int      `json:"processed_count"`
}

// singleCheckpoint is the resume state for single-repo mode.
type singleCheckpoint struct {
	ProcessedCount int `json:"processed_count"`
}

// FetchStream implements connector.StreamProvider.
// When repo is empty, all repos accessible to the token are auto-discovered.
// SourceDocumentID format: "{owner}/{repo}/{path}" in all modes for uniqueness.
func (c *Connector) FetchStream(ctx context.Context, cfg map[string]any, opts connector.FetchOpts, emit func(connector.Document) error) error {
	token, _ := cfg["token"].(string)
	owner, _ := cfg["owner"].(string)
	repo, _ := cfg["repo"].(string)
	branch, _ := cfg["branch"].(string)

	if token == "" {
		return fmt.Errorf("github: token is required")
	}

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

	// Resolve owner from token if not provided.
	if owner == "" {
		res, err := do("https://api.github.com/user")
		if err != nil {
			return fmt.Errorf("github: resolve token owner: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("github: resolve token owner returned HTTP %d", res.StatusCode)
		}
		var u struct{ Login string `json:"login"` }
		if err := json.NewDecoder(res.Body).Decode(&u); err != nil {
			return fmt.Errorf("github: decode user info: %w", err)
		}
		owner = u.Login
	}

	if repo != "" {
		// Single-repo mode.
		return c.syncRepo(ctx, do, owner, repo, branch, opts, emit)
	}

	// Multi-repo mode: discover all repos accessible to the token.
	log := slog.With("connector", "github", "owner", owner)

	var cp multiCheckpoint
	if len(opts.Checkpoint) > 2 {
		_ = json.Unmarshal(opts.Checkpoint, &cp)
	}
	completedSet := make(map[string]bool, len(cp.CompletedRepos))
	for _, r := range cp.CompletedRepos {
		completedSet[r] = true
	}

	// Paginate all accessible repos.
	var repos []string
	page := 1
	for {
		u := fmt.Sprintf("https://api.github.com/user/repos?per_page=100&page=%d&sort=full_name", page)
		res, err := do(u)
		if err != nil {
			return fmt.Errorf("github: list repos: %w", err)
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return fmt.Errorf("github: list repos returned HTTP %d", res.StatusCode)
		}
		var batch []struct {
			FullName string `json:"full_name"`
		}
		_ = json.NewDecoder(res.Body).Decode(&batch)
		res.Body.Close()
		for _, r := range batch {
			repos = append(repos, r.FullName)
		}
		if len(batch) < 100 {
			break
		}
		page++
	}
	log.Info("discovered repos", "count", len(repos), "already_completed", len(completedSet))

	for _, fullName := range repos {
		if completedSet[fullName] {
			log.Info("skipping completed repo", "repo", fullName)
			continue
		}

		parts := strings.SplitN(fullName, "/", 2)
		if len(parts) != 2 {
			continue
		}
		repoOwner, repoName := parts[0], parts[1]

		// Within a resumed repo, pass the per-repo processed_count.
		repoOpts := connector.FetchOpts{}
		if fullName == cp.CurrentRepo && cp.ProcessedCount > 0 {
			b, _ := json.Marshal(singleCheckpoint{ProcessedCount: cp.ProcessedCount})
			repoOpts.Checkpoint = b
			log.Info("resuming repo from checkpoint", "repo", fullName, "offset", cp.ProcessedCount)
		}
		savedCP := cp // capture for closure
		savedFull := fullName
		repoOpts.SaveCheckpoint = func(v any) {
			if opts.SaveCheckpoint == nil {
				return
			}
			sc, ok := v.(map[string]any)
			if !ok {
				return
			}
			pc, _ := sc["processed_count"].(int)
			opts.SaveCheckpoint(multiCheckpoint{
				CompletedRepos: savedCP.CompletedRepos,
				CurrentRepo:    savedFull,
				ProcessedCount: pc,
			})
		}

		if err := c.syncRepo(ctx, do, repoOwner, repoName, branch, repoOpts, emit); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Warn("repo sync failed, continuing", "repo", fullName, "error", err)
		}

		// Mark repo as completed.
		cp.CompletedRepos = append(cp.CompletedRepos, fullName)
		completedSet[fullName] = true
		cp.CurrentRepo = ""
		cp.ProcessedCount = 0
		if opts.SaveCheckpoint != nil {
			opts.SaveCheckpoint(multiCheckpoint{
				CompletedRepos: cp.CompletedRepos,
			})
		}
	}

	log.Info("multi-repo sync complete", "repos", len(repos))
	return nil
}

// syncRepo fetches and emits all text files from a single repository.
// SourceDocumentID = "{owner}/{repo}/{path}" for uniqueness across multi-repo syncs.
func (c *Connector) syncRepo(
	ctx context.Context,
	do func(string) (*http.Response, error),
	owner, repo, branch string,
	opts connector.FetchOpts,
	emit func(connector.Document) error,
) error {
	fullName := owner + "/" + repo
	log := slog.With("connector", "github", "repo", fullName)

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

	type blob struct {
		path string
		size int
	}
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

	// Load resume offset.
	var cp singleCheckpoint
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
			Source:           "github:" + fullName,
			SourceDocumentID: fullName + "/" + b.path,
			Title:            fullName + "/" + b.path,
			URL:              fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", owner, repo, branch, b.path),
			Author:           owner,
			Content:          text,
			Metadata:         map[string]any{"repo": fullName},
		}); err != nil {
			return err
		}

		processed++
		if opts.SaveCheckpoint != nil && processed%10 == 0 {
			opts.SaveCheckpoint(map[string]any{"processed_count": i + 1})
		}
	}

	log.Info("repo sync complete", "repo", fullName, "files_processed", processed)
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
