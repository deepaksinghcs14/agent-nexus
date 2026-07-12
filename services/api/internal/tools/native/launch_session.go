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

// LaunchRepoSessionTool starts a headless Claude Code session against a repo
// via the runner service and blocks the run (durably — see WaitForSession)
// until the session's completion callback arrives.
type LaunchRepoSessionTool struct {
	tools.RequiresRunContext
	pool           *pgxpool.Pool
	cfg            *config.Config
	runnerURL      string
	callbackURL    string
	callbackSecret string
}

func NewLaunchRepoSessionTool(pool *pgxpool.Pool, cfg *config.Config) *LaunchRepoSessionTool {
	callbackBase := cfg.SessionCallbackURL
	if callbackBase == "" {
		callbackBase = cfg.PublicAPIURL
	}
	return &LaunchRepoSessionTool{
		pool:           pool,
		cfg:            cfg,
		runnerURL:      strings.TrimRight(cfg.RunnerURL, "/"),
		callbackURL:    strings.TrimRight(callbackBase, "/") + "/internal/sessions/callback",
		callbackSecret: cfg.RunnerCallbackSecret,
	}
}

// runnerTokens returns the workspace's decrypted pipeline credentials
// (stored via the runner-credentials API), shared by every tool that launches
// a runner session. Either may be "" — the runner and tools then fall back to
// instance env.
func runnerTokens(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, workspaceID string) (claude, github string) {
	var encClaude, encGithub *string
	if err := pool.QueryRow(ctx,
		`SELECT claude_token, github_token FROM runner_credentials WHERE workspace_id=$1::uuid`, workspaceID).
		Scan(&encClaude, &encGithub); err != nil {
		return "", ""
	}
	dec := func(enc *string) string {
		if enc == nil || *enc == "" {
			return ""
		}
		v, err := encrypt.Decrypt([]byte(cfg.EncryptionKey), *enc)
		if err != nil {
			return ""
		}
		return v
	}
	return dec(encClaude), dec(encGithub)
}

// checkSessionsEnabled enforces the repo-onboarding gate shared by every tool
// that launches a runner session: sessions may only target repositories that
// are onboarded AND have sessions explicitly enabled. Connector syncs make
// repos known (searchable); the enable toggle in Settings → Claude Code is the
// deliberate act that grants session access — so a hallucinated or merely
// indexed repo fails here mechanically, with the real options listed.
func checkSessionsEnabled(ctx context.Context, pool *pgxpool.Pool, workspaceID, repo string) error {
	var known, enabled bool
	if err := pool.QueryRow(ctx, `
		SELECT true, sessions_enabled FROM repo_catalog
		WHERE workspace_id=$1::uuid AND repo=$2`,
		workspaceID, repo).Scan(&known, &enabled); err != nil {
		known = false
	}
	if enabled {
		return nil
	}
	rows, err := pool.Query(ctx,
		`SELECT repo FROM repo_catalog WHERE workspace_id=$1::uuid AND sessions_enabled ORDER BY repo`, workspaceID)
	var allowed []string
	if err == nil {
		for rows.Next() {
			var r string
			if rows.Scan(&r) == nil {
				allowed = append(allowed, r)
			}
		}
		rows.Close()
	}
	if known {
		return fmt.Errorf("repository %q is indexed but sessions are not enabled for it — a workspace admin can enable it in Settings → Claude Code → Repositories", repo)
	}
	if len(allowed) == 0 {
		return fmt.Errorf("repository %q is not onboarded and no repositories have sessions enabled — a workspace admin can add/enable repositories in Settings → Claude Code", repo)
	}
	return fmt.Errorf("repository %q is not onboarded in this workspace — sessions can only target enabled repos (%s); a workspace admin can add it in Settings → Claude Code", repo, strings.Join(allowed, ", "))
}

// launchAndWait POSTs a launch payload to the runner and blocks (durably
// parking, for top-level runs) until the session's completion callback,
// returning the callback content as the tool output.
func launchAndWait(ctx context.Context, execCtx tools.ExecutionContext, runnerURL string, launch map[string]any, waitKey string) (any, error) {
	payload, _ := json.Marshal(launch)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, runnerURL+"/sessions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runner unreachable: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("runner rejected session launch: %s: %s", res.Status, strings.TrimSpace(string(b)))
	}

	content, err := execCtx.WaitForSession(ctx, waitKey)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if json.Unmarshal([]byte(content), &out) != nil {
		return map[string]any{"raw": content}, nil
	}
	return out, nil
}

func (t *LaunchRepoSessionTool) Definition() domain.Tool {
	return domain.Tool{
		Name: "native_launch_repo_session",
		Description: "Launch an autonomous Claude Code coding session against a GitHub repository. " +
			"The session clones the repo, works on the given task on a fresh branch, pushes the branch, " +
			"and this tool returns its outcome: {status: success|budget-exceeded|crashed, branch, summary, cost_usd}. " +
			"Sessions can take minutes to hours; execution waits for completion. " +
			"Call once per (ticket, repo) pair — repeat calls for the same pair return the existing session.",
		Type: "native",
		InputSchema: json.RawMessage(`{"type":"object","properties":{
			"repo":{"type":"string","description":"GitHub repository as owner/name, e.g. deepaksinghcs14/agent-nexus"},
			"ticket_key":{"type":"string","description":"Jira ticket key this work belongs to, e.g. PROJ-123"},
			"task_description":{"type":"string","description":"Complete, self-contained description of the work to do in this repo"},
			"base_branch":{"type":"string","description":"Branch to start from (default: the repo default branch)"},
			"budget_usd":{"type":"number","description":"Optional cost cap for the session in USD"}
		},"required":["repo","ticket_key","task_description"]}`),
		RiskLevel: "high",
		TimeoutMs: 120000,
	}
}

func (t *LaunchRepoSessionTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	repo, _ := input["repo"].(string)
	ticketKey, _ := input["ticket_key"].(string)
	task, _ := input["task_description"].(string)
	baseBranch, _ := input["base_branch"].(string)
	budgetUSD, _ := input["budget_usd"].(float64)

	var bad []string
	bad = checkRepoArg(bad, repo)
	if ticketKey == "" {
		bad = append(bad, "ticket_key is missing")
	}
	if task == "" {
		bad = append(bad, "task_description is missing")
	}
	if len(bad) > 0 {
		return nil, badArgsError(bad)
	}
	if t.runnerURL == "" {
		return nil, fmt.Errorf("repo sessions are not configured (RUNNER_URL is unset)")
	}
	if execCtx.WaitForSession == nil {
		return nil, fmt.Errorf("repo sessions are not available in this run context")
	}

	if err := checkSessionsEnabled(ctx, t.pool, execCtx.WorkspaceID, repo); err != nil {
		return nil, err
	}

	// Inject accumulated review lessons for this repo so the session doesn't
	// repeat mistakes past reviews already flagged.
	var lessons string
	if err := t.pool.QueryRow(ctx,
		`SELECT lessons FROM repo_catalog WHERE workspace_id=$1::uuid AND repo=$2`,
		execCtx.WorkspaceID, repo).Scan(&lessons); err == nil && strings.TrimSpace(lessons) != "" {
		task += "\n\nLessons from past sessions and reviews in this repo — avoid repeating these:\n" + lessons
	}

	launch := map[string]any{
		"run_id":           execCtx.RunID,
		"repo":             repo,
		"ticket_key":       ticketKey,
		"task_description": task,
		"base_branch":      baseBranch,
		"budget_usd":       budgetUSD,
		"callback_url":     t.callbackURL,
		"callback_secret":  t.callbackSecret,
	}
	// Workspace credentials take precedence over any static keys configured
	// on the runner service itself — GitHub access in particular is a
	// workspace concern, not an instance one.
	claudeToken, githubToken := runnerTokens(ctx, t.pool, t.cfg, execCtx.WorkspaceID)
	if claudeToken != "" {
		launch["claude_token"] = claudeToken
	}
	if githubToken != "" {
		launch["github_token"] = githubToken
	}
	return launchAndWait(ctx, execCtx, t.runnerURL, launch, ticketKey+"|"+repo)
}
