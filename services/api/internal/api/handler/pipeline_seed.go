package handler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The three Jira→PR pipeline agents are seeded into every workspace as
// protected (non-deletable) system agents, so pipeline setup never has to
// create them: SeedPipelineAgents runs at startup for existing workspaces and
// per-workspace on registration / workspace creation. Seeding is idempotent
// by agent name and never overwrites user edits — an agent is only created
// when the workspace has no agent with that name.

const orchestratorInstructions = `You are the autonomous Jira-to-PR pipeline orchestrator. Input is a Jira ticket (key, summary, description). Drive it to pull requests without human interaction.

Procedure:
1. UNDERSTAND — if Jira/Confluence tools are attached, fetch the full ticket and linked pages for complete context.
2. SELECT REPOS — search the repo catalog with native_retrieve_context from several angles (feature area, module names, technical terms). Choose the target repositories (usually one). For each, write a complete, self-contained task description carrying ALL relevant ticket context — the coding session cannot see this conversation.
3. ANNOUNCE — if a Jira comment tool is attached, post the plan (repos + task summaries) as a ticket comment.
4. EXECUTE — call native_launch_repo_session once per (ticket, repo). The call returns when the session finishes (it may take hours).
5. OUTCOMES — success: proceed. budget-exceeded: post a Jira comment noting partial progress on the returned branch, keep going with other repos, and only open a PR for that repo if the summary says the work is complete. crashed: retry once with a sharper task description; on a second crash, post a Jira comment and move on.
6. REVIEW — for each successful branch call native_get_branch_diff, then send the diff plus the ticket context to the agent named "Code Review Agent" via native_call_agent. If it blocks, launch ONE follow-up session with the blocking issues as the task, then re-review once.
7. OPEN PRS — native_create_pull_request per repo: head is the session branch, title "[TICKET-KEY] summary", body covering the ticket, the changes, and the review notes.
8. CLOSE OUT — post a final Jira comment with PR links; transition the ticket to "In Review" if a transition tool is attached.

Rules: never invent repository names — only repos found in the catalog. If the catalog yields nothing relevant, say so in a Jira comment and stop. One session per (ticket, repo) — repeated calls join the running session.`

const reviewInstructions = `You are a code review agent in an autonomous delivery pipeline. You receive a unified diff (and ticket context) as input. Review it for: correctness bugs, missing error handling, security issues, and divergence from the stated task. Respond ONLY with JSON: {"verdict":"approve"|"block","blocking_issues":[{"file":"...","issue":"...","suggestion":"..."}],"non_blocking_notes":["..."],"pr_description_notes":"one paragraph summarizing the change for the PR description"}. Block only for real defects — style preferences are non-blocking notes.`

const docsMapInstructions = `You maintain llms.txt documentation maps for repositories. Input tells you which repository changed (and optionally which pull request merged). Call native_launch_repo_session exactly once with: the repo, ticket_key set to "DOCS-MAP", and a task description instructing the session to (1) read the repository structure and recent changes, (2) create or update llms.txt at the repo root as a concise navigation map for AI agents — key entry points, module responsibilities, where to make common kinds of changes, (3) commit directly with message "docs: update llms.txt map". Report the session outcome as your final answer.`

const (
	PipelineOrchestratorName = "Jira Pipeline Orchestrator"
	PipelineReviewerName     = "Code Review Agent"
	PipelineDocsMapName      = "Docs Map Maintainer"
)

type pipelineAgentSpec struct {
	name         string
	description  string
	instructions string
	maxTokens    int
	maxSteps     int
	agenticRAG   bool
	toolNames    []string
}

var pipelineAgentSpecs = []pipelineAgentSpec{
	{
		name:         PipelineOrchestratorName,
		description:  "Webhook-triggered Jira ticket to PR pipeline (protected system agent)",
		instructions: orchestratorInstructions,
		maxTokens:    8192,
		maxSteps:     60,
		agenticRAG:   true,
		toolNames: []string{
			"native_launch_repo_session", "native_create_pull_request",
			"native_get_branch_diff", "native_call_agent",
		},
	},
	{
		name:         PipelineReviewerName,
		description:  "Reviews pipeline branch diffs before PRs are opened (protected system agent)",
		instructions: reviewInstructions,
		maxTokens:    4096,
		maxSteps:     10,
	},
	{
		name:         PipelineDocsMapName,
		description:  "Keeps per-repo llms.txt maps fresh after merges and on schedule (protected system agent)",
		instructions: docsMapInstructions,
		maxTokens:    4096,
		maxSteps:     10,
		toolNames:    []string{"native_launch_repo_session"},
	},
}

// SeedPipelineAgents ensures every workspace has the three pipeline agents.
// Safe to call on every startup.
func SeedPipelineAgents(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT id::text, owner_id::text FROM workspaces`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type ws struct{ id, owner string }
	var all []ws
	for rows.Next() {
		var w ws
		if rows.Scan(&w.id, &w.owner) == nil {
			all = append(all, w)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	seeded := 0
	for _, w := range all {
		n, err := SeedPipelineAgentsForWorkspace(ctx, pool, w.id, w.owner)
		if err != nil {
			slog.Warn("pipeline agent seeding failed for workspace", "workspace_id", w.id, "error", err)
			continue
		}
		seeded += n
	}
	if seeded > 0 {
		slog.Info("pipeline agents seeded", "created", seeded)
	}
	return nil
}

// SeedPipelineAgentsForWorkspace creates any of the three pipeline agents the
// workspace is missing (matched by name) and returns how many were created.
func SeedPipelineAgentsForWorkspace(ctx context.Context, pool *pgxpool.Pool, workspaceID, ownerID string) (int, error) {
	// Inherit the workspace's prevailing provider/model so seeded agents run
	// without extra configuration; fall back to Anthropic defaults.
	provider, model := "anthropic", "claude-sonnet-4-6"
	_ = pool.QueryRow(ctx, `
		SELECT provider, model FROM agents
		WHERE workspace_id=$1::uuid AND status='active' AND protected=false
		ORDER BY created_at LIMIT 1`, workspaceID).Scan(&provider, &model)

	created := 0
	for _, spec := range pipelineAgentSpecs {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM agents WHERE workspace_id=$1::uuid AND name=$2)`,
			workspaceID, spec.name).Scan(&exists); err != nil {
			return created, err
		}
		if exists {
			continue
		}

		agentID := uuid.NewString()
		if _, err := pool.Exec(ctx, `
			INSERT INTO agents(
				id, workspace_id, name, description, instructions,
				provider, model, temperature, max_tokens,
				memory_enabled, memory_scope, context_retrieval_enabled, agentic_rag,
				max_steps, max_tool_calls, max_duration_secs, status, created_by,
				tags, max_memories, min_relevance_score, memory_save_mode,
				memory_review_policy, memory_min_importance, memory_dedupe_threshold,
				max_history_messages, lazy_tool_loading, ephemeral, protected,
				compaction_threshold, compaction_token_threshold
			) VALUES (
				$1::uuid, $2::uuid, $3, $4, $5,
				$6, $7, 0, $8,
				false, 'conversation', $9, $9,
				$10, 100, 7200, 'active', $11::uuid,
				'{}', 20, 0.7, 'hybrid',
				'uncertain', 0.9, 0.88,
				20, false, false, true,
				10, 8000
			)`,
			agentID, workspaceID, spec.name, spec.description, spec.instructions,
			provider, model, spec.maxTokens,
			spec.agenticRAG,
			spec.maxSteps, ownerID); err != nil {
			return created, fmt.Errorf("seed %s: %w", spec.name, err)
		}

		if len(spec.toolNames) > 0 {
			if _, err := pool.Exec(ctx, `
				INSERT INTO agent_tools(agent_id, tool_id, enabled)
				SELECT $1::uuid, id, true FROM tools
				WHERE name = ANY($2) AND workspace_id IS NULL
				ON CONFLICT DO NOTHING`,
				agentID, spec.toolNames); err != nil {
				return created, fmt.Errorf("attach tools to %s: %w", spec.name, err)
			}
		}

		// Link the workspace's repo catalog to the orchestrator when it
		// already exists (catalog-ingest also links it on onboarding).
		if spec.name == PipelineOrchestratorName {
			_, _ = pool.Exec(ctx, `
				INSERT INTO agent_connectors(agent_id, connector_id, enabled, max_chunks, min_score)
				SELECT $1::uuid, id, true, 10, 0.3 FROM connectors
				WHERE workspace_id=$2::uuid AND name='repo-catalog'
				ON CONFLICT (agent_id, connector_id) DO NOTHING`,
				agentID, workspaceID)
		}
		created++
	}
	return created, nil
}
