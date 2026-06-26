package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

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

type Connector struct{}

func New() *Connector { return &Connector{} }

func (c *Connector) Fetch(ctx context.Context, cfg map[string]any) ([]connector.Document, error) {
	token, _ := cfg["token"].(string)
	owner, _ := cfg["owner"].(string)
	repo, _ := cfg["repo"].(string)
	branch, _ := cfg["branch"].(string)

	if token == "" || owner == "" || repo == "" {
		return nil, fmt.Errorf("github connector: token, owner, and repo are required")
	}

	do := func(url string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		return http.DefaultClient.Do(req)
	}

	if branch == "" {
		res, err := do(fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo))
		if err != nil {
			return nil, fmt.Errorf("github connector: failed to fetch repo info: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("github connector: repo info returned %d", res.StatusCode)
		}
		var info struct {
			DefaultBranch string `json:"default_branch"`
		}
		if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
			return nil, fmt.Errorf("github connector: failed to decode repo info: %w", err)
		}
		branch = info.DefaultBranch
	}

	// Get full recursive file tree
	treeURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, branch)
	res, err := do(treeURL)
	if err != nil {
		return nil, fmt.Errorf("github connector: failed to fetch tree: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("github connector: tree returned %d", res.StatusCode)
	}

	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			Size int    `json:"size"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(res.Body).Decode(&tree); err != nil {
		return nil, fmt.Errorf("github connector: failed to decode tree: %w", err)
	}

	var docs []connector.Document
	for _, item := range tree.Tree {
		if item.Type != "blob" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(item.Path))
		if binaryExts[ext] {
			continue
		}
		if item.Size > maxFileBytes {
			continue
		}

		contentURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, item.Path, branch)
		cres, err := do(contentURL)
		if err != nil {
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

		docs = append(docs, connector.Document{
			Source:           "github",
			SourceDocumentID: item.Path,
			Title:            filepath.Base(item.Path),
			URL:              fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", owner, repo, branch, item.Path),
			Author:           owner,
			Content:          text,
		})
	}

	return docs, nil
}
