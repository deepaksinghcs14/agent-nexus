-- Seed the "Agent Self-Management" managed skill.
-- Enabling this skill on an agent automatically attaches all 10 self-management
-- tools via the required_tool_names mechanism in repository/skills.go.
INSERT INTO skills (id, workspace_id, name, description, content, source, enabled, required_tool_names)
VALUES (
  gen_random_uuid(),
  NULL,
  'Agent Self-Management',
  'Enables the agent to call other agents, create/destroy agents, skills, and HTTP tools at runtime.',
  E'## Self-Management Capabilities\nYou can dynamically create, call, and destroy resources during this run:\n\n**Agents**\n- `native_list_agents` — list existing agents in the workspace\n- `native_call_agent(agent_id, task)` — delegate a task to another agent and get its output\n- `native_create_agent(name, instructions, provider?, model?, tool_names[]?, ephemeral?)` — spin up a new agent\n- `native_delete_agent(agent_id)` — remove an agent you created in this run\n\n**Skills**\n- `native_list_skills` — list available skills\n- `native_create_skill(name, content, attach_to_self?, ephemeral?)` — create a skill; set attach_to_self=true to inject its content into your own context\n- `native_delete_skill(skill_id)` — remove a skill you created in this run\n\n**HTTP Tools**\n- `native_list_http_tools` — list HTTP tools in the workspace\n- `native_create_http_tool(name, url, method?, headers?, input_schema?, ephemeral?)` — register an external API as a callable tool\n- `native_delete_tool(tool_id)` — remove an HTTP tool you created in this run\n\n**Guidance**\n- Issue multiple `native_call_agent` calls in a single response to run sub-agents in parallel (wall-clock = slowest, not sum).\n- Set `ephemeral=true` on any resource that should auto-delete when this run ends.\n- Use `native_create_skill` with `attach_to_self=true` to encode domain knowledge and inject it into your own context for the remainder of this run.\n- Max recursion depth is 3; `native_call_agent` will return an error if the chain is too deep.',
  'managed',
  true,
  ARRAY[
    'native_list_agents',
    'native_call_agent',
    'native_create_agent',
    'native_delete_agent',
    'native_list_skills',
    'native_create_skill',
    'native_delete_skill',
    'native_list_http_tools',
    'native_create_http_tool',
    'native_delete_tool'
  ]
)
ON CONFLICT DO NOTHING;
