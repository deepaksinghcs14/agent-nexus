#!/usr/bin/env bash
# Sets up the autonomous Jira→PR pipeline on an Agent Nexus instance:
# orchestrator + review + docs-map agents, tool attachments, the repo-catalog
# connector link, and the Jira / GitHub webhook triggers.
#
# Prerequisites:
#   - Agent Nexus API running with RUNNER_URL, RUNNER_CALLBACK_SECRET, and
#     GITHUB_TOKEN configured
#   - An LLM provider credential configured in the workspace
#   - Repos onboarded via: go run ./services/api/cmd/catalog-ingest -repo ... -workspace ...
#   - (Optional) Atlassian MCP server connected via the OAuth flow; attach its
#     jira/confluence tools to the orchestrator afterwards
#
# Usage:
#   NEXUS_API=http://localhost:8080 NEXUS_TOKEN=<jwt> \
#   PROVIDER=anthropic MODEL=claude-sonnet-4-6 \
#   JIRA_LABEL=auto-dev ./infra/scripts/setup_pipeline.sh
set -euo pipefail

export NEXUS_API="${NEXUS_API:-http://localhost:8080}"
export NEXUS_TOKEN="${NEXUS_TOKEN:?NEXUS_TOKEN (JWT or API token) is required}"
export PROVIDER="${PROVIDER:-anthropic}"
export MODEL="${MODEL:-claude-sonnet-4-6}"
export JIRA_LABEL="${JIRA_LABEL:-auto-dev}"
export JIRA_WEBHOOK_SECRET="${JIRA_WEBHOOK_SECRET:-}"
export GITHUB_WEBHOOK_SECRET="${GITHUB_WEBHOOK_SECRET:-}"

api() { # method path [json-body]
  local method="$1" path="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -sf -X "$method" "$NEXUS_API/api/v1$path" \
      -H "Authorization: Bearer $NEXUS_TOKEN" -H 'Content-Type: application/json' -d "$body"
  else
    curl -sf -X "$method" "$NEXUS_API/api/v1$path" -H "Authorization: Bearer $NEXUS_TOKEN"
  fi
}

id_of() { python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])'; }

# ── agent instruction texts (passed to python via env, no shell quoting games) ─

export REVIEW_INSTRUCTIONS='You are a code review agent in an autonomous delivery pipeline. You receive a unified diff (and ticket context) as input. Review it for: correctness bugs, missing error handling, security issues, and divergence from the stated task. Respond ONLY with JSON: {"verdict":"approve"|"block","blocking_issues":[{"file":"...","issue":"...","suggestion":"..."}],"non_blocking_notes":["..."],"pr_description_notes":"one paragraph summarizing the change for the PR description"}. Block only for real defects — style preferences are non-blocking notes.'

export DOCS_INSTRUCTIONS='You maintain llms.txt documentation maps for repositories. Input tells you which repository changed (and optionally which pull request merged). Call native_launch_repo_session exactly once with: the repo, ticket_key set to "DOCS-MAP", and a task description instructing the session to (1) read the repository structure and recent changes, (2) create or update llms.txt at the repo root as a concise navigation map for AI agents — key entry points, module responsibilities, where to make common kinds of changes, (3) commit directly with message "docs: update llms.txt map". Report the session outcome as your final answer.'

export ORCH_INSTRUCTIONS='You are the autonomous Jira-to-PR pipeline orchestrator. Input is a Jira ticket (key, summary, description). Drive it to pull requests without human interaction.

Procedure:
1. UNDERSTAND — if Jira/Confluence tools are attached, fetch the full ticket and linked pages for complete context.
2. SELECT REPOS — search the repo catalog with native_retrieve_context from several angles (feature area, module names, technical terms). Choose the target repositories (usually one). For each, write a complete, self-contained task description carrying ALL relevant ticket context — the coding session cannot see this conversation.
3. ANNOUNCE — if a Jira comment tool is attached, post the plan (repos + task summaries) as a ticket comment.
4. EXECUTE — call native_launch_repo_session once per (ticket, repo). The call returns when the session finishes (it may take hours).
5. OUTCOMES — success: proceed. budget-exceeded: post a Jira comment noting partial progress on the returned branch, keep going with other repos, and only open a PR for that repo if the summary says the work is complete. crashed: retry once with a sharper task description; on a second crash, post a Jira comment and move on.
6. REVIEW — for each successful branch call native_get_branch_diff, then send the diff plus the ticket context to the agent named "Code Review Agent" via native_call_agent. If it blocks, launch ONE follow-up session with the blocking issues as the task, then re-review once.
7. OPEN PRS — native_create_pull_request per repo: head is the session branch, title "[TICKET-KEY] summary", body covering the ticket, the changes, and the review notes.
8. CLOSE OUT — post a final Jira comment with PR links; transition the ticket to "In Review" if a transition tool is attached.

Rules: never invent repository names — only repos found in the catalog. If the catalog yields nothing relevant, say so in a Jira comment and stop. One session per (ticket, repo) — repeated calls join the running session.'

payload() { # builds an agent-create JSON body from env vars
  python3 - "$@" <<'PYEOF'
import json, os, sys
kind = sys.argv[1]
base = {'provider': os.environ['PROVIDER'], 'model': os.environ['MODEL'], 'temperature': 0}
if kind == 'review':
    base |= {'name': 'Code Review Agent', 'description': 'Reviews pipeline branch diffs before PRs are opened',
             'instructions': os.environ['REVIEW_INSTRUCTIONS'], 'max_tokens': 4096, 'max_steps': 10}
elif kind == 'docs':
    base |= {'name': 'Docs Map Maintainer', 'description': 'Keeps per-repo llms.txt maps fresh after merges and on schedule',
             'instructions': os.environ['DOCS_INSTRUCTIONS'], 'max_tokens': 4096, 'max_steps': 10}
elif kind == 'orch':
    base |= {'name': 'Jira Pipeline Orchestrator', 'description': 'Webhook-triggered Jira ticket to PR pipeline',
             'instructions': os.environ['ORCH_INSTRUCTIONS'], 'max_tokens': 8192, 'max_steps': 60,
             'context_retrieval_enabled': True, 'agentic_rag': True}
print(json.dumps(base))
PYEOF
}

trigger_payload() { # kind target_id
  python3 - "$@" <<'PYEOF'
import json, os, sys
kind, target = sys.argv[1], sys.argv[2]
label = os.environ['JIRA_LABEL']
if kind == 'jira':
    tpl = ('{{- $match := false -}}{{- range .Body.issue.fields.labels -}}'
           '{{- if eq . "' + label + '" -}}{{- $match = true -}}{{- end -}}{{- end -}}'
           '{{- if $match }}Jira ticket {{.Body.issue.key}}: {{.Body.issue.fields.summary}}\n\n'
           '{{.Body.issue.fields.description}}{{ end -}}')
    body = {'name': 'jira-auto-dev', 'description': f'Fires the pipeline for Jira tickets labeled {label}',
            'target_type': 'agent', 'target_id': target, 'secret': os.environ['JIRA_WEBHOOK_SECRET'],
            'input_template': tpl}
else:
    tpl = ('{{- if and (eq .Body.action "closed") .Body.pull_request.merged }}'
           'Pull request #{{.Body.pull_request.number}} ("{{.Body.pull_request.title}}") merged into '
           '{{.Body.repository.full_name}} (base {{.Body.pull_request.base.ref}}). '
           'Update the llms.txt docs map for this repository.{{ end -}}')
    body = {'name': 'github-merge-docs-map', 'description': 'Refreshes the repo docs map after merges',
            'target_type': 'agent', 'target_id': target, 'secret': os.environ['GITHUB_WEBHOOK_SECRET'],
            'input_template': tpl}
print(json.dumps(body))
PYEOF
}

echo "== Review agent =="
REVIEW_ID=$(api POST /agents "$(payload review)" | id_of)
echo "review agent: $REVIEW_ID"

echo "== Docs-map agent =="
DOCS_ID=$(api POST /agents "$(payload docs)" | id_of)
api PUT "/agents/$DOCS_ID/tools" '{"tool_names":["native_launch_repo_session"]}' > /dev/null
echo "docs-map agent: $DOCS_ID"

echo "== Orchestrator agent =="
ORCH_ID=$(api POST /agents "$(payload orch)" | id_of)
api PUT "/agents/$ORCH_ID/tools" '{"tool_names":["native_launch_repo_session","native_create_pull_request","native_get_branch_diff","native_call_agent"]}' > /dev/null
echo "orchestrator agent: $ORCH_ID"

echo "== Link repo catalog connector =="
CONN_ID=$(api GET /connectors | python3 -c "
import sys,json
for c in json.load(sys.stdin)['data']:
    if c['name']=='repo-catalog': print(c['id']); break")
if [ -n "$CONN_ID" ]; then
  api PUT "/agents/$ORCH_ID/connectors" "{\"connector_ids\":[\"$CONN_ID\"],\"max_chunks\":10,\"min_score\":0.3}" > /dev/null
  echo "linked connector: $CONN_ID"
else
  echo "WARNING: no repo-catalog connector found — run catalog-ingest first, then: PUT /agents/$ORCH_ID/connectors"
fi

echo "== Jira webhook trigger (label filter: $JIRA_LABEL) =="
JIRA_TRIG=$(api POST /webhook-triggers "$(trigger_payload jira "$ORCH_ID")" | id_of)
echo "jira trigger webhook URL: $NEXUS_API/webhook/$JIRA_TRIG"

echo "== GitHub merged-PR trigger (docs map) =="
GH_TRIG=$(api POST /webhook-triggers "$(trigger_payload github "$DOCS_ID")" | id_of)
echo "github trigger webhook URL: $NEXUS_API/webhook/$GH_TRIG"

cat <<EOF

== Done ==
Remaining manual steps:
 1. Atlassian: create the MCP server (url https://mcp.atlassian.com/v1/mcp),
    click "Connect (OAuth)" on the MCP Servers page (or POST
    /api/v1/mcp-servers/{id}/oauth/start), then attach the synced jira
    comment/transition tools to agent $ORCH_ID.
 2. Point a Jira webhook (issue created/updated) at the jira trigger URL above.
 3. Point a GitHub org/repo webhook (pull_request events) at the github trigger URL.
 4. Weekly docs-map refresh: schedule (e.g. Railway cron)
      curl -X POST $NEXUS_API/api/v1/invoke/agents/$DOCS_ID \\
        -H "Authorization: Bearer \$API_TOKEN" -H 'Content-Type: application/json' \\
        -d '{"input":"Scheduled refresh: regenerate the llms.txt docs map for every repository in the catalog.","stream":false}'
EOF
