-- Prefix native tool names with 'native_' so tool type is visually distinguishable
-- from MCP tools in the agent builder, run traces, and SSE events.
-- LLM APIs only allow [a-zA-Z0-9_-] in tool names so underscore is used as separator.

-- Rename native tools (workspace_id IS NULL = global/system tools)
UPDATE tools
  SET name = 'native_' || name
  WHERE type = 'native'
    AND workspace_id IS NULL
    AND name NOT LIKE 'native_%';

-- Prefix MCP tool names in the mcp_tools registry
UPDATE mcp_tools
  SET name = 'mcp_' || name
  WHERE name NOT LIKE 'mcp_%';

-- Prefix MCP tool names in the shared tools table
UPDATE tools
  SET name = 'mcp_' || name
  WHERE type = 'mcp'
    AND name NOT LIKE 'mcp_%';

-- Update global policy: approval_required_tools references old native tool names
UPDATE policies
  SET value = REPLACE(
                REPLACE(value::text, '"write_file"', '"native_write_file"'),
                '"http_request"', '"native_http_request"'
              )::jsonb
  WHERE key = 'approval_required_tools'
    AND workspace_id IS NULL;
