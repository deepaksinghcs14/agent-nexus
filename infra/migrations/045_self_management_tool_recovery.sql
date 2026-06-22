UPDATE skills
SET content = content || '

**When a needed tool is not in your toolset — self-recovery (MANDATORY):**
- NEVER give up or say "I do not have the tool". Always recover:
  1. Call `native_list_workspace_tools` to see every tool available in the workspace
  2. Call `native_request_tool(tool_name)` to attach it to yourself for this run
  3. Then call the tool normally
- This applies to `http_request`, any MCP tool, any HTTP tool — if it exists in the workspace you can get it.
- Only fall back to `native_create_http_tool` if the tool truly does not exist in the workspace yet.',
    updated_at = NOW()
WHERE name = 'Agent Self-Management'
  AND content NOT LIKE '%self-recovery%';
