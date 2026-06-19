-- Migration 032: Expanded Agent Self-Management
-- Adds source_run_id + ephemeral to workflows table for ownership enforcement.
-- Updates Agent Self-Management skill with 11 new tool names and expanded content.

ALTER TABLE workflows ADD COLUMN IF NOT EXISTS source_run_id UUID REFERENCES runs(id) ON DELETE SET NULL;
ALTER TABLE workflows ADD COLUMN IF NOT EXISTS ephemeral BOOLEAN NOT NULL DEFAULT false;

-- Update required_tool_names for Agent Self-Management skill
UPDATE skills
SET required_tool_names = ARRAY[
  'native_list_agents','native_call_agent','native_create_agent','native_delete_agent',
  'native_list_skills','native_create_skill','native_delete_skill',
  'native_list_http_tools','native_create_http_tool','native_delete_tool',
  'native_update_agent','native_attach_skill','native_detach_skill','native_update_skill',
  'native_list_workspace_tools','native_attach_tool','native_detach_tool',
  'native_create_code_tool',
  'native_list_workflows','native_create_workflow','native_run_workflow','native_delete_workflow'
],
content = content || E'\n\n**Full Agent Control**\n- `native_update_agent(agent_id, name?, description?, instructions?, model?, provider?, temperature?, max_tokens?, max_steps?, memory_enabled?, memory_scope?, status?)` — update any agent\'s settings\n- `native_attach_skill(agent_id, skill_name)` / `native_detach_skill(agent_id, skill_id)` — attach or detach skills on any agent\n- `native_attach_tool(agent_id, tool_name)` / `native_detach_tool(agent_id, tool_name)` — attach or detach tools on any agent\n- `native_update_skill(skill_id, name?, description?, content?)` — update a skill you created\n- `native_list_workspace_tools` — list all tools (native + HTTP + MCP + code) in the workspace\n\n**Code Tools**\n- `native_create_code_tool(name, description, code, input_schema?)` — create a custom JavaScript tool that runs in a sandbox. The `code` is a function body receiving `input` and returning a value. No network or filesystem access. Auto-attached to you on creation.\n\n**Workflows**\n- `native_list_workflows` — list all workflows\n- `native_create_workflow(name, mode, agent_ids[]?)` — create a pipeline or supervisor workflow\n- `native_run_workflow(workflow_id, input)` — trigger a workflow run in the background, returns run_id\n- `native_delete_workflow(workflow_id)` — delete a workflow you created in this run'
WHERE name = 'Agent Self-Management' AND workspace_id IS NULL;
