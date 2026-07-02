package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/deepaksingh/agent-nexus/services/api/pkg/encrypt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GitHub REST tools for the Jira→PR pipeline: open pull requests from
// runner-pushed branches and read branch diffs for the review stage.
// Token resolution is workspace-first (Settings → Claude Code), with the
// instance-level GITHUB_TOKEN env as a single-tenant fallback.

var githubHTTP = &http.Client{Timeout: 30 * time.Second}

// githubTokenFor resolves the effective GitHub token for a workspace.
func githubTokenFor(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, workspaceID string) string {
	if pool != nil && workspaceID != "" {
		var enc *string
		if err := pool.QueryRow(ctx,
			`SELECT github_token FROM runner_credentials WHERE workspace_id=$1::uuid`, workspaceID).Scan(&enc); err == nil && enc != nil && *enc != "" {
			if token, err := encrypt.Decrypt([]byte(cfg.EncryptionKey), *enc); err == nil {
				return token
			}
		}
	}
	return cfg.GithubToken
}

func githubRequest(ctx context.Context, apiURL, token, method, path string, body any) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(apiURL, "/")+path, rd)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := githubHTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	return res.StatusCode, raw, nil
}

func validRepo(repo string) bool {
	return strings.Count(repo, "/") == 1 && !strings.Contains(repo, "..") && !strings.ContainsAny(repo, " \t\n")
}

// ── native_create_pull_request ────────────────────────────────────────────────

type CreatePullRequestTool struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewCreatePullRequestTool(pool *pgxpool.Pool, cfg *config.Config) *CreatePullRequestTool {
	return &CreatePullRequestTool{pool: pool, cfg: cfg}
}

func (t *CreatePullRequestTool) Definition() domain.Tool {
	return domain.Tool{
		Name: "native_create_pull_request",
		Description: "Open a GitHub pull request from an existing branch. " +
			"Returns {number, url, state}. If a PR already exists for the branch, returns that PR instead of failing.",
		Type: "native",
		InputSchema: json.RawMessage(`{"type":"object","properties":{
			"repo":{"type":"string","description":"Repository as owner/name"},
			"head":{"type":"string","description":"Branch with the changes, e.g. nexus/PROJ-123"},
			"base":{"type":"string","description":"Target branch (default: main)"},
			"title":{"type":"string","description":"Pull request title"},
			"body":{"type":"string","description":"Pull request description (markdown)"}
		},"required":["repo","head","title"]}`),
		RiskLevel: "high",
		TimeoutMs: 30000,
	}
}

func (t *CreatePullRequestTool) Execute(input map[string]any) (any, error) {
	return t.run(context.Background(), t.cfg.GithubToken, input)
}

func (t *CreatePullRequestTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	return t.run(ctx, githubTokenFor(ctx, t.pool, t.cfg, execCtx.WorkspaceID), input)
}

func (t *CreatePullRequestTool) run(ctx context.Context, token string, input map[string]any) (any, error) {
	repo, _ := input["repo"].(string)
	head, _ := input["head"].(string)
	base, _ := input["base"].(string)
	title, _ := input["title"].(string)
	body, _ := input["body"].(string)
	if !validRepo(repo) || head == "" || title == "" {
		return nil, fmt.Errorf("repo (owner/name), head, and title are required")
	}
	if token == "" {
		return nil, fmt.Errorf("no GitHub token: set one in Settings → Claude Code, or GITHUB_TOKEN on the API")
	}
	if base == "" {
		base = "main"
	}

	status, raw, err := githubRequest(ctx, t.cfg.GithubAPIURL, token, http.MethodPost,
		"/repos/"+repo+"/pulls",
		map[string]any{"title": title, "head": head, "base": base, "body": body})
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnprocessableEntity && bytes.Contains(raw, []byte("already exists")) {
		// Idempotency: surface the existing PR for this head branch.
		owner := strings.SplitN(repo, "/", 2)[0]
		s2, raw2, err2 := githubRequest(ctx, t.cfg.GithubAPIURL, token, http.MethodGet,
			"/repos/"+repo+"/pulls?state=open&head="+owner+":"+head, nil)
		if err2 == nil && s2 == http.StatusOK {
			var prs []struct {
				Number  int    `json:"number"`
				HTMLURL string `json:"html_url"`
				State   string `json:"state"`
			}
			if json.Unmarshal(raw2, &prs) == nil && len(prs) > 0 {
				return map[string]any{"number": prs[0].Number, "url": prs[0].HTMLURL, "state": prs[0].State, "existing": true}, nil
			}
		}
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("github: create PR failed (%d): %s", status, truncateBytes(raw, 400))
	}
	var pr struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		State   string `json:"state"`
	}
	if err := json.Unmarshal(raw, &pr); err != nil {
		return nil, fmt.Errorf("github: unexpected PR response")
	}
	return map[string]any{"number": pr.Number, "url": pr.HTMLURL, "state": pr.State}, nil
}

// ── native_get_branch_diff ────────────────────────────────────────────────────

type GetBranchDiffTool struct {
	pool *pgxpool.Pool
	cfg  *config.Config
}

func NewGetBranchDiffTool(pool *pgxpool.Pool, cfg *config.Config) *GetBranchDiffTool {
	return &GetBranchDiffTool{pool: pool, cfg: cfg}
}

func (t *GetBranchDiffTool) Definition() domain.Tool {
	return domain.Tool{
		Name: "native_get_branch_diff",
		Description: "Get the diff between two branches of a GitHub repository (files changed with patches, truncated for large diffs). " +
			"Use to review changes on a branch before opening a pull request.",
		Type: "native",
		InputSchema: json.RawMessage(`{"type":"object","properties":{
			"repo":{"type":"string","description":"Repository as owner/name"},
			"head":{"type":"string","description":"Branch with the changes"},
			"base":{"type":"string","description":"Branch to compare against (default: main)"}
		},"required":["repo","head"]}`),
		RiskLevel: "low",
		TimeoutMs: 30000,
	}
}

func (t *GetBranchDiffTool) Execute(input map[string]any) (any, error) {
	return t.run(context.Background(), t.cfg.GithubToken, input)
}

func (t *GetBranchDiffTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	return t.run(ctx, githubTokenFor(ctx, t.pool, t.cfg, execCtx.WorkspaceID), input)
}

func (t *GetBranchDiffTool) run(ctx context.Context, token string, input map[string]any) (any, error) {
	repo, _ := input["repo"].(string)
	head, _ := input["head"].(string)
	base, _ := input["base"].(string)
	if !validRepo(repo) || head == "" {
		return nil, fmt.Errorf("repo (owner/name) and head are required")
	}
	if token == "" {
		return nil, fmt.Errorf("no GitHub token: set one in Settings → Claude Code, or GITHUB_TOKEN on the API")
	}
	if base == "" {
		base = "main"
	}

	status, raw, err := githubRequest(ctx, t.cfg.GithubAPIURL, token, http.MethodGet,
		"/repos/"+repo+"/compare/"+base+"..."+head, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("github: compare failed (%d): %s", status, truncateBytes(raw, 400))
	}
	var cmp struct {
		TotalCommits int `json:"total_commits"`
		Files        []struct {
			Filename  string `json:"filename"`
			Status    string `json:"status"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
			Patch     string `json:"patch"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &cmp); err != nil {
		return nil, fmt.Errorf("github: unexpected compare response")
	}

	const perFilePatch = 6000
	const totalBudget = 60000
	used := 0
	files := make([]map[string]any, 0, len(cmp.Files))
	for _, f := range cmp.Files {
		patch := f.Patch
		if len(patch) > perFilePatch {
			patch = patch[:perFilePatch] + "\n…[patch truncated]"
		}
		if used+len(patch) > totalBudget {
			patch = "[omitted — total diff budget reached]"
		}
		used += len(patch)
		files = append(files, map[string]any{
			"filename": f.Filename, "status": f.Status,
			"additions": f.Additions, "deletions": f.Deletions, "patch": patch,
		})
	}
	return map[string]any{"total_commits": cmp.TotalCommits, "files_changed": len(cmp.Files), "files": files}, nil
}

func truncateBytes(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
