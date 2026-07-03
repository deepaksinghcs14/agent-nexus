-- Workspace-level Jira credentials for the native Jira tools (get issue, JQL
-- search, comment, transition). API-token auth for orgs whose Atlassian site
-- blocks MCP OAuth: Cloud uses email + API token (Basic), Data Center uses a
-- PAT with email left empty (Bearer). Token stored AES-256-GCM encrypted like
-- the Claude/GitHub credentials; base URL and email are not secrets.
ALTER TABLE runner_credentials ADD COLUMN IF NOT EXISTS jira_base_url TEXT;
ALTER TABLE runner_credentials ADD COLUMN IF NOT EXISTS jira_email TEXT;
ALTER TABLE runner_credentials ADD COLUMN IF NOT EXISTS jira_api_token TEXT;
