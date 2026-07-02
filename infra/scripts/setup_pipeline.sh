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
# The three pipeline agents (Jira Pipeline Orchestrator, Code Review Agent,
# Docs Map Maintainer) are seeded automatically as protected system agents —
# this script only wires the webhook triggers and the catalog connector.
# Adjust the agents' provider/model in the UI if the seeded defaults
# (workspace's prevailing provider, else anthropic/claude-sonnet-4-6) don't fit.
#
# Usage:
#   NEXUS_API=http://localhost:8080 NEXUS_TOKEN=<jwt> \
#   JIRA_LABEL=auto-dev ./infra/scripts/setup_pipeline.sh
set -euo pipefail

export NEXUS_API="${NEXUS_API:-http://localhost:8080}"
export NEXUS_TOKEN="${NEXUS_TOKEN:?NEXUS_TOKEN (JWT or API token) is required}"
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

# The three pipeline agents are seeded automatically (protected system agents)
# on registration / workspace creation / API startup — look them up by name.
echo "== Locate seeded pipeline agents =="
AGENTS_JSON=$(api GET /agents)
agent_id_by_name() {
  echo "$AGENTS_JSON" | python3 -c "
import sys, json
for a in json.load(sys.stdin)['data']:
    if a['name'] == '$1': print(a['id']); break"
}
ORCH_ID=$(agent_id_by_name 'Jira Pipeline Orchestrator')
DOCS_ID=$(agent_id_by_name 'Docs Map Maintainer')
REVIEW_ID=$(agent_id_by_name 'Code Review Agent')
if [ -z "$ORCH_ID" ] || [ -z "$DOCS_ID" ] || [ -z "$REVIEW_ID" ]; then
  echo "ERROR: seeded pipeline agents not found — restart the API (seeding runs at startup) and re-run." >&2
  exit 1
fi
echo "orchestrator: $ORCH_ID"
echo "review:       $REVIEW_ID"
echo "docs-map:     $DOCS_ID"

echo "== Link repo catalog connector (idempotent; catalog-ingest also links it) =="
CONN_ID=$(api GET /connectors | python3 -c "
import sys,json
for c in json.load(sys.stdin)['data']:
    if c['name']=='repo-catalog': print(c['id']); break")
if [ -n "$CONN_ID" ]; then
  api PUT "/agents/$ORCH_ID/connectors" "{\"connector_ids\":[\"$CONN_ID\"],\"max_chunks\":10,\"min_score\":0.3}" > /dev/null
  echo "linked connector: $CONN_ID"
else
  echo "NOTE: no repo-catalog connector yet — run catalog-ingest; it links the connector automatically."
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
