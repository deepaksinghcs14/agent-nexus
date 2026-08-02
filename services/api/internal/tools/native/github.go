package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

// maxRateLimitRetryWait bounds the in-call wait when the quota window resets
// soon: a near reset is worth riding out (the run is already executing);
// anything longer is surfaced as a clear error instead of a silent stall.
const maxRateLimitRetryWait = 2 * time.Minute

func githubRequest(ctx context.Context, apiURL, token, method, path string, body any) (int, []byte, error) {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		payload = b
	}
	for attempt := 0; ; attempt++ {
		var rd io.Reader
		if payload != nil {
			rd = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(apiURL, "/")+path, rd)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := githubHTTP.Do(req)
		if err != nil {
			return 0, nil, err
		}
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()

		if rateLimited(res, raw) {
			reset := rateLimitReset(res)
			wait := time.Until(reset)
			if attempt == 0 && wait > 0 && wait <= maxRateLimitRetryWait {
				select {
				case <-ctx.Done():
					return 0, nil, ctx.Err()
				case <-time.After(wait + 5*time.Second):
					continue
				}
			}
			return res.StatusCode, raw, fmt.Errorf(
				"GitHub rate limit exceeded for this token — resets at %s (in %s). Background indexing shares the same hourly quota; retry after the reset",
				reset.Local().Format("15:04:05"), wait.Round(time.Minute))
		}
		return res.StatusCode, raw, nil
	}
}

// rateLimited reports whether a response was rejected for REST quota
// exhaustion (403/429 with the remaining header at zero, or GitHub's
// rate-limit message in the body).
func rateLimited(res *http.Response, body []byte) bool {
	if res.StatusCode != http.StatusForbidden && res.StatusCode != http.StatusTooManyRequests {
		return false
	}
	if res.Header.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	return bytes.Contains(body, []byte("rate limit exceeded"))
}

// rateLimitReset parses X-RateLimit-Reset (epoch seconds), defaulting to an
// hour out when absent.
func rateLimitReset(res *http.Response) time.Time {
	if v, err := strconv.ParseInt(res.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && v > 0 {
		return time.Unix(v, 0)
	}
	return time.Now().Add(time.Hour)
}

func validRepo(repo string) bool {
	return strings.Count(repo, "/") == 1 && !strings.Contains(repo, "..") && !strings.ContainsAny(repo, " \t\n")
}

// badArgsError joins per-field validation problems into one error that echoes
// what was received, so the calling agent can fix the arguments and retry
// instead of guessing which field a joint "X and Y are required" refers to.
func badArgsError(problems []string) error {
	return fmt.Errorf("invalid arguments — fix and retry: %s", strings.Join(problems, "; "))
}

// checkRepoArg validates the owner/name form, appending an echo of the
// received value on failure.
func checkRepoArg(problems []string, repo string) []string {
	if !validRepo(repo) {
		return append(problems, fmt.Sprintf(`repo must be "owner/name" (got %q)`, repo))
	}
	return problems
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
		// Default false so the unattended Jira→PR pipeline still opens PRs;
		// REQUIRE_SESSION_APPROVAL=true adds a human gate. See the matching
		// comment in launch_session.go.
		RequiresApproval: t.cfg.RequireSessionApproval,
		TimeoutMs:        30000,
		Enabled:          true,
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
	var bad []string
	bad = checkRepoArg(bad, repo)
	if head == "" {
		bad = append(bad, "head (the branch with the changes) is missing")
	}
	if title == "" {
		bad = append(bad, "title is missing")
	}
	if len(bad) > 0 {
		return nil, badArgsError(bad)
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
	var bad []string
	bad = checkRepoArg(bad, repo)
	if head == "" {
		bad = append(bad, "head (the branch to diff) is missing")
	}
	if len(bad) > 0 {
		return nil, badArgsError(bad)
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
