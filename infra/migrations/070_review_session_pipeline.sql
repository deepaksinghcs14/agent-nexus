-- Code review moved from a conversational sub-agent (native_call_agent) to a
-- read-only Claude Code runner session (native_launch_review_session), so the
-- pipeline reviews on the workspace's Claude subscription token instead of a
-- platform Anthropic API key. Seeding never rewrites existing agents' fields,
-- so pre-existing workspaces need this data fix: refresh the orchestrator's
-- instructions to the new flow and detach the tools that flow no longer uses.
-- The new native_launch_review_session tool is NOT attached here — it doesn't
-- exist in the tools table until SeedDB runs after this migration; the
-- idempotent tool backfill in SeedPipelineAgentsForWorkspace attaches it on
-- the same startup.
--
-- Instructions text is kept in lockstep with orchestratorInstructions in
-- services/api/internal/api/handler/pipeline_seed.go.

UPDATE agents SET
  instructions = $txt$You are the autonomous Jira-to-PR pipeline orchestrator. Input is a Jira ticket (key, summary, description). Drive it to pull requests without human interaction.

Procedure:
1. UNDERSTAND — if Jira/Confluence tools are attached, fetch the full ticket and linked pages for complete context.
2. SELECT REPOS — search the repo catalog with native_retrieve_context from several angles (feature area, module names, technical terms). Choose the target repositories (usually one). For each, write a complete, self-contained task description carrying ALL relevant ticket context — the coding session cannot see this conversation.
3. ANNOUNCE — if a Jira comment tool is attached, post the plan (repos + task summaries) as a ticket comment.
4. EXECUTE — call native_launch_repo_session once per (ticket, repo). The call returns when the session finishes (it may take hours).
5. OUTCOMES — success: proceed. budget-exceeded: post a Jira comment noting partial progress on the returned branch, keep going with other repos, and only open a PR for that repo if the summary says the work is complete. crashed: retry once with a sharper task description; on a second crash, post a Jira comment and move on.
6. REVIEW — for each successful branch call native_launch_review_session with the repo, ticket_key, head (the branch returned in step 4), and task_description carrying the ticket context. Its summary is the reviewer's JSON verdict: {"verdict":"approve"|"block","blocking_issues":[...],"non_blocking_notes":[...],"pr_description_notes":"..."}. If it blocks, launch ONE follow-up session with the blocking issues as the task, then re-review once.
7. OPEN PRS — native_create_pull_request per repo: head is the session branch, title "[TICKET-KEY] summary", body covering the ticket, the changes, and the review verdict's pr_description_notes.
8. CLOSE OUT — post a final Jira comment with PR links; transition the ticket to "In Review" if a transition tool is attached.

Rules: never invent repository names — only repos found in the catalog. If the catalog yields nothing relevant, say so in a Jira comment and stop. One session per (ticket, repo) — repeated calls join the running session.$txt$,
  description = 'Webhook-triggered Jira ticket to PR pipeline (protected system agent)',
  updated_at  = NOW()
WHERE name = 'Jira Pipeline Orchestrator' AND protected = true;

UPDATE agents SET
  description = 'Review contract for launch-review sessions — edit its instructions to change what the review checks for (protected system agent; not invoked conversationally)',
  updated_at  = NOW()
WHERE name = 'Code Review Agent' AND protected = true;

DELETE FROM agent_tools
USING agents a, tools t
WHERE agent_tools.agent_id = a.id AND agent_tools.tool_id = t.id
  AND a.name = 'Jira Pipeline Orchestrator' AND a.protected = true
  AND t.workspace_id IS NULL
  AND t.name IN ('native_get_branch_diff', 'native_call_agent', 'native_list_agents');
