# Autonomous Jira → PR Pipeline

A Jira ticket labeled `auto-dev` is driven to reviewed pull requests with no
human step other than tool approvals: repo selection via the repo catalog,
headless Claude Code sessions in the runner service, an automated review pass,
PR creation, and Jira updates.

```
Jira webhook ──▶ /webhook/{id} ──▶ Orchestrator agent run
  (label filter in the           │ 1. read ticket (Atlassian MCP, OAuth)
   trigger's input_template)     │ 2. native_retrieve_context → repo catalog
                                 │ 3. native_launch_repo_session ──▶ runner service
                                 │      (run parks durably in session_wait;      │ git clone (GITHUB_TOKEN)
                                 │       callback resumes it — survives restarts)│ claude -p … (ANTHROPIC_API_KEY)
                                 │ ◀── POST /internal/sessions/callback ─────────┘ push nexus/<ticket>
                                 │ 4. native_get_branch_diff + review agent (native_call_agent)
                                 │ 5. native_create_pull_request
                                 └ 6. Jira comment / transition
```

## Components

| Piece | Where |
|---|---|
| Durable waits (approval + session) | `run_wait_states` table; park/resume in `services/api/internal/api/handler/{wait_state,session_wait,resume}.go` |
| Runner service (Railway container) | `services/runner` — `RUNNER_EXECUTOR=claude\|stub`, idempotent per `(ticket, repo)` |
| Repo catalog | `services/api/cmd/catalog-ingest` → `connectors` named `repo-catalog` + `repo_catalog` table |
| GitHub tools | `native_create_pull_request`, `native_get_branch_diff` (`GITHUB_TOKEN`, `GITHUB_API_URL`) |
| Session tool | `native_launch_repo_session` (`RUNNER_URL`, `RUNNER_CALLBACK_SECRET`, `PUBLIC_API_URL`) |
| Atlassian MCP (hosted, OAuth 2.1) | `POST /api/v1/mcp-servers/{id}/oauth/start` or the “Connect (OAuth)” button; tokens auto-refresh |
| Pipeline assembly | `infra/scripts/setup_pipeline.sh` (agents, connector link, webhook triggers) |
| Docs maps (llms.txt) | Docs Map Maintainer agent; post-merge GitHub trigger + scheduled invoke |

## Setup

1. **Deploy** the API (env: `RUNNER_URL`, `RUNNER_CALLBACK_SECRET`, `GITHUB_TOKEN`)
   and the runner (`services/runner/Dockerfile`; env: `GITHUB_TOKEN`,
   `RUNNER_EXECUTOR=claude`). For Claude auth, either connect a **Claude
   account** in Settings → Providers (run `claude setup-token` locally, paste
   the token — stored encrypted, injected per session, subscription billing)
   or set `ANTHROPIC_API_KEY` / `CLAUDE_CODE_OAUTH_TOKEN` on the runner as a
   static fallback. The per-workspace account takes precedence.
2. **Onboard repos**:
   `DATABASE_URL=… GITHUB_TOKEN=… go run ./services/api/cmd/catalog-ingest -repo owner/name -workspace <uuid>`
3. **Assemble the pipeline**:
   `NEXUS_API=… NEXUS_TOKEN=… PROVIDER=anthropic MODEL=claude-sonnet-4-6 ./infra/scripts/setup_pipeline.sh`
   — prints the Jira and GitHub webhook URLs to configure.
4. **Connect Atlassian**: add an MCP server with URL
   `https://mcp.atlassian.com/v1/mcp`, click **Connect (OAuth)**, grant access,
   then attach the synced Jira comment/transition tools to the orchestrator.
5. **Schedule** the weekly docs-map refresh (the setup script prints the
   `curl` for a Railway cron).

## Operational notes

- A run waiting on an approval or a session **survives API restarts**: state is
  persisted in `run_wait_states` and the run resumes on the approval decision
  or the runner callback. Approvals for parked runs are served from the run
  detail page (no live playground needed).
- Budget fallback: the runner reports `budget-exceeded` with the partial
  branch; the orchestrator posts a Jira comment and continues with other repos.
- Sessions are idempotent per `(ticket, repo)`; a duplicate launch joins the
  in-flight session. Runner idempotency state is in-memory — run one replica.
- A runner crash/restart mid-session does not strand runs: the runner journals
  every in-flight session (on its volume) and delivers `crashed` callbacks for
  leftovers at startup, and the API's session watchdog resumes any
  `session_wait` run whose callback never arrives within
  `SESSION_WAIT_TIMEOUT_MIN` (default 240; keep it above the runner's
  `SESSION_TIMEOUT_MIN`). The orchestrator's crash handling (retry once, then
  report to Jira) takes over from there.
- Native-tool `requires_approval` flags are reset from code defaults on every
  API startup (`SeedDB`) — gate pipeline approvals on http/code/MCP tools or
  set the flag in the Go registry definition.

## Local integration testing (no external credentials)

`infra/scripts/` ships three mocks used by the E2E flows:
`mock_llm.py` (OpenAI-compatible; `MOCK_SCRIPT` drives multi-turn runs),
`mock_github.py` (PR create + compare), `mock_oauth_mcp.py` (OAuth 2.1 AS +
Bearer-gated MCP server). Run the runner with `RUNNER_EXECUTOR=stub`, ingest a
local checkout with `catalog-ingest -clone-url <path>`, run
`setup_pipeline.sh` against the local API, and POST a Jira-shaped payload to
the printed webhook URL.
