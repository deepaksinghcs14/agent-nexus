package native

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deepaksingh/agent-nexus/services/api/internal/config"
	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LaunchReviewSessionTool runs a read-only Claude Code review session against
// an existing branch via the runner service: the runner checks the branch out,
// diffs it against base, and returns the reviewer's verdict JSON as the
// session summary. Review runs on the workspace's Claude subscription token
// (the same credential coding sessions use) — no platform API key involved.
type LaunchReviewSessionTool struct {
	tools.RequiresRunContext
	pool           *pgxpool.Pool
	cfg            *config.Config
	runnerURL      string
	callbackURL    string
	callbackSecret string
}

func NewLaunchReviewSessionTool(pool *pgxpool.Pool, cfg *config.Config) *LaunchReviewSessionTool {
	callbackBase := cfg.SessionCallbackURL
	if callbackBase == "" {
		callbackBase = cfg.PublicAPIURL
	}
	return &LaunchReviewSessionTool{
		pool:           pool,
		cfg:            cfg,
		runnerURL:      strings.TrimRight(cfg.RunnerURL, "/"),
		callbackURL:    strings.TrimRight(callbackBase, "/") + "/internal/sessions/callback",
		callbackSecret: cfg.RunnerCallbackSecret,
	}
}

// fallbackReviewContract is used only when the workspace's "Code Review Agent"
// row (whose instructions are the editable review contract) is missing. Keep
// the JSON shape identical to the seeded reviewInstructions in
// pipeline_seed.go — the handler package can't be imported here (cycle).
const fallbackReviewContract = `Review the changes for: correctness bugs, missing error handling, security issues, and divergence from the stated task. Respond ONLY with JSON: {"verdict":"approve"|"block","blocking_issues":[{"file":"...","issue":"...","suggestion":"..."}],"non_blocking_notes":["..."],"pr_description_notes":"one paragraph summarizing the change for the PR description"}. Block only for real defects — style preferences are non-blocking notes.`

func (t *LaunchReviewSessionTool) Definition() domain.Tool {
	return domain.Tool{
		Name: "native_launch_review_session",
		Description: "Review an existing branch with a read-only Claude Code session. " +
			"The session checks out the branch, diffs it against base, and returns the review verdict JSON in its summary: " +
			`{status, branch, summary: '{"verdict":"approve"|"block","blocking_issues":[...],"non_blocking_notes":[...],"pr_description_notes":"..."}', cost_usd}. ` +
			"Nothing is committed or pushed. Execution waits for completion.",
		Type: "native",
		InputSchema: json.RawMessage(`{"type":"object","properties":{
			"repo":{"type":"string","description":"GitHub repository as owner/name"},
			"ticket_key":{"type":"string","description":"Jira ticket key this review belongs to, e.g. PROJ-123"},
			"head":{"type":"string","description":"Existing branch to review, e.g. nexus/PROJ-123"},
			"base":{"type":"string","description":"Branch to diff against (default: main)"},
			"task_description":{"type":"string","description":"Ticket context so the reviewer can judge divergence from the stated task"},
			"budget_usd":{"type":"number","description":"Optional cost cap for the review session in USD"}
		},"required":["repo","ticket_key","head"]}`),
		RiskLevel: "low",
		TimeoutMs: 120000,
	}
}

func (t *LaunchReviewSessionTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	repo, _ := input["repo"].(string)
	ticketKey, _ := input["ticket_key"].(string)
	head, _ := input["head"].(string)
	base, _ := input["base"].(string)
	task, _ := input["task_description"].(string)
	budgetUSD, _ := input["budget_usd"].(float64)

	if repo == "" || ticketKey == "" || head == "" {
		return nil, fmt.Errorf("repo, ticket_key, and head are required")
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

	// The workspace's "Code Review Agent" holds the editable review contract
	// in its instructions — it is configuration for this tool, not an agent
	// that runs conversationally.
	contract := fallbackReviewContract
	var instructions string
	if err := t.pool.QueryRow(ctx,
		`SELECT instructions FROM agents WHERE workspace_id=$1::uuid AND name='Code Review Agent'`,
		execCtx.WorkspaceID).Scan(&instructions); err == nil && strings.TrimSpace(instructions) != "" {
		contract = instructions
	}
	taskDescription := contract
	if strings.TrimSpace(task) != "" {
		taskDescription += "\n\nTicket context:\n" + task
	}

	launch := map[string]any{
		"run_id":           execCtx.RunID,
		"mode":             "review",
		"repo":             repo,
		"ticket_key":       ticketKey,
		"head":             head,
		"base_branch":      base,
		"task_description": taskDescription,
		"budget_usd":       budgetUSD,
		"callback_url":     t.callbackURL,
		"callback_secret":  t.callbackSecret,
	}
	claudeToken, githubToken := runnerTokens(ctx, t.pool, t.cfg, execCtx.WorkspaceID)
	if claudeToken != "" {
		launch["claude_token"] = claudeToken
	}
	if githubToken != "" {
		launch["github_token"] = githubToken
	}
	// The wait key is a label (callbacks route by run_id) but mirrors the
	// runner's mode-aware dedup key so SSE events and logs line up.
	return launchAndWait(ctx, execCtx, t.runnerURL, launch, "review:"+ticketKey+"|"+repo)
}
