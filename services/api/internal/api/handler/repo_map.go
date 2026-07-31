package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools/native"
)

// Repo map: a cached, terse architecture summary for a repo (directory
// layout, key modules/entrypoints, conventions). native_launch_repo_session
// injects it into the task instead of letting the session re-explore the
// same structure from a cold clone every ticket, and asks the model to emit
// a "### Repo Map" section in its final summary whenever the cache is
// missing or stale. recordRepoMap persists that section, replacing the old
// snapshot outright — unlike lessons, a map isn't a log, it's a single
// current picture of the repo.

// maxRepoMap caps the stored map text.
const maxRepoMap = 4000

// distillRepoMap extracts the "### Repo Map" section from a session summary,
// or "" if the model didn't include one.
func distillRepoMap(summary string) string {
	const marker = "### Repo Map"
	i := strings.Index(summary, marker)
	if i < 0 {
		return ""
	}
	body := summary[i+len(marker):]
	if j := strings.Index(body, "\n### "); j >= 0 {
		body = body[:j]
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if len(body) > maxRepoMap {
		body = body[:maxRepoMap]
	}
	return body
}

// recordRepoMap persists a coding session's repo map, if it included one.
// Best-effort: a failure here must never fail the runner callback.
func recordRepoMap(ctx context.Context, pool *pgxpool.Pool, runID, repo, summary string) {
	block := distillRepoMap(summary)
	if block == "" {
		return
	}
	var workspaceID string
	if err := pool.QueryRow(ctx, `
		SELECT r.workspace_id::text
		FROM runs r JOIN repo_catalog rc ON rc.workspace_id = r.workspace_id AND rc.repo = $2
		WHERE r.id = $1::uuid`, runID, repo).Scan(&workspaceID); err != nil {
		slog.Warn("repo map: no catalog row for session callback", "run_id", runID, "repo", repo, "error", err)
		return
	}
	if _, err := pool.Exec(ctx,
		`UPDATE repo_catalog SET repo_map=$3, repo_map_updated_at=NOW(), updated_at=NOW() WHERE workspace_id=$1::uuid AND repo=$2`,
		workspaceID, repo, block); err != nil {
		slog.Warn("repo map: failed to persist", "run_id", runID, "repo", repo, "error", err)
	}
}

// ── On-demand generation ──────────────────────────────────────────────────────
//
// The automatic path only (re)generates a repo's map as a side effect of the
// next real coding session on it. These let a user trigger that survey
// immediately, for one repo or every session-enabled repo, without waiting
// for a ticket.

// mapGenerationTicketKey is the fixed, reused ticket key for on-demand map
// sessions — distinct from real Jira tickets, and reused across requests so
// repeated clicks reset the same throwaway branch instead of piling up new
// ones.
const mapGenerationTicketKey = "REPO-MAP"

// mapGenerationTask is the fixed task handed to the session: a pure survey,
// no code changes, explicitly overriding any cached map native_launch_repo_session
// would otherwise inject (this call's whole point is to refresh it).
func mapGenerationTask(repo string) string {
	return fmt.Sprintf(
		"Survey the repository %s and produce a fresh architecture map. "+
			"Do not modify, add, or delete any files — this is a read-only survey, not a coding task. "+
			"If a cached map is provided below, ignore it: this request is specifically to refresh it "+
			"against the repository's current state.\n\n"+
			"Append a `### Repo Map` section to your final summary: a terse (at most 30 lines) bullet "+
			"overview of the directory layout, key modules/entrypoints, and repo-specific conventions "+
			"a new contributor should know.", repo)
}

// launchMapGenerationRun creates a throwaway run/conversation (for Runs-list
// visibility and to anchor the session's workspace_id/recordRepoMap join) and,
// in a background goroutine, calls native_launch_repo_session directly — no
// LLM turn decides the args, they're fixed — to (re)generate repo's cached
// map. Returns the run_id immediately; the goroutine outlives the request.
func (h *InvokeHandler) launchMapGenerationRun(ctx context.Context, ws, uid, agentID, repo string) (string, error) {
	convID := uuid.NewString()
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO conversations(id,workspace_id,agent_id,user_id,title) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5)`,
		convID, ws, agentID, uid, "Repo map: "+repo); err != nil {
		return "", fmt.Errorf("create conversation: %w", err)
	}
	task := mapGenerationTask(repo)
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO messages(id,conversation_id,role,content) VALUES($1::uuid,$2::uuid,'user',$3)`,
		uuid.NewString(), convID, task); err != nil {
		return "", err
	}
	runID := uuid.NewString()
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO runs(id,workspace_id,agent_id,conversation_id,user_id,input,status) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,'running')`,
		runID, ws, agentID, convID, uid, task); err != nil {
		return "", fmt.Errorf("create run: %w", err)
	}

	go func() {
		bgCtx := context.Background()
		execCtx := tools.ExecutionContext{
			WorkspaceID:    ws,
			RunID:          runID,
			WaitForSession: h.blockingSessionWait(runID, nil),
		}
		launchTool := native.NewLaunchRepoSessionTool(h.pool, h.cfg)
		out, err := launchTool.ExecuteWithContext(bgCtx, execCtx, map[string]any{
			"repo":             repo,
			"ticket_key":       mapGenerationTicketKey,
			"task_description": task,
		})
		status, output := "success", ""
		if err != nil {
			status, output = "failed", err.Error()
		} else if b, mErr := json.Marshal(out); mErr == nil {
			output = string(b)
		}
		if _, err := h.pool.Exec(bgCtx,
			`UPDATE runs SET status=$2, output=$3, completed_at=NOW() WHERE id=$1::uuid`,
			runID, status, output); err != nil {
			slog.Warn("repo map: failed to finalize generation run", "run_id", runID, "repo", repo, "error", err)
		}
	}()

	return runID, nil
}
