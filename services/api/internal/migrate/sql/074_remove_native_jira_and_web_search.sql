-- Removes the native Jira tools and their dedicated credential columns (superseded
-- by the MCP Atlassian preset), and the native web-search tool (superseded by the
-- MCP Brave Search preset). agent_tools rows referencing these tools cascade-delete
-- via their FK to tools(id).
DELETE FROM tools WHERE workspace_id IS NULL AND name IN (
  'native_jira_get_issue', 'native_jira_search', 'native_jira_add_comment', 'native_jira_transition_issue',
  'native_web_search'
);

ALTER TABLE runner_credentials DROP COLUMN IF EXISTS jira_base_url;
ALTER TABLE runner_credentials DROP COLUMN IF EXISTS jira_email;
ALTER TABLE runner_credentials DROP COLUMN IF EXISTS jira_api_token;
