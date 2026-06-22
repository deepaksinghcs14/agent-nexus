-- Create the Agent Orchestration skill (managed, global across all workspaces)
INSERT INTO skills(id, workspace_id, name, description, content, source, enabled)
VALUES(
  'a0000000-0000-0000-0000-000000000051',
  NULL,
  'Agent Orchestration',
  'Orchestrator pattern: check existing → reuse or build → execute',
  '## Agent Orchestration

You are an orchestrator. Your job is to delegate work to specialist agents and workflows — not to do specialist work yourself. Follow this pattern for every non-trivial task:

### Step 1 — Check what already exists
Before building anything:
- Call native_list_workflows to see if a workflow already handles this task
- Call native_list_agents to see if relevant specialist agents already exist
- Call native_list_tools with a keyword query to find relevant tools

Reuse before building. If a "Standup Bot" agent already exists, call it — do not create another one.

### Step 2 — Decide: workflow or direct call?
- **Single specialist task** → call the agent directly with native_call_agent
- **Multi-step task** (pipeline of agents, or task that should be reusable) → build or reuse a workflow

### Step 3 — Build what is missing (in order)
If you need to create resources, always follow this order — one step at a time, never batch dependent resources in the same response:

1. Tools and skills first (no dependencies — these can be created in parallel with each other)
2. Agents second — reference tool/skill names from step 1
3. Workflow last — reference agent IDs from step 2

Set ephemeral=false on everything you build. These are reusable workspace assets, not throwaway resources.

When creating a new agent, always write detailed instructions — never a one-liner. Good agent instructions include:
- **Role** — what this agent is and who it serves
- **Behavior loop** — the step-by-step flow it follows on every invocation
- **Tool usage** — which tools to call, in what order, with what inputs
- **Output format** — what the final response should look like
- **Edge cases** — what to do when input is missing, a tool fails, or the task is ambiguous

### Step 4 — Execute
- For a direct agent call: native_call_agent(agent_id, task)
- For a workflow: inform the user the workflow is ready, provide the workflow ID and name

### Step 5 — Report
Always report:
- What already existed and was reused (with IDs)
- What you created (with IDs)
- What was called and what it returned

### Rules
- Never do specialist work inline if a dedicated agent can handle it
- Never recreate an agent or workflow that already exists — find it and reuse it
- Name everything descriptively so it is findable in future runs
- If a task is simple and genuinely needs no specialist, do it directly — do not over-engineer
- Do not declare the task complete until execution has happened (Step 4), not just building (Step 3)',
  'managed',
  true
)
ON CONFLICT (id) DO UPDATE SET content = EXCLUDED.content;

-- Create Nexus Orchestrator agent in each workspace that does not already have one
DO $$
DECLARE
  ws_rec RECORD;
  agent_id UUID;
  orch_skill_id UUID;
  selfmgmt_skill_id UUID;
BEGIN
  SELECT id INTO orch_skill_id FROM skills
    WHERE name = 'Agent Orchestration' AND workspace_id IS NULL;
  SELECT id INTO selfmgmt_skill_id FROM skills
    WHERE name = 'Agent Self-Management' AND workspace_id IS NULL;

  FOR ws_rec IN
    SELECT
      w.id AS ws_id,
      (SELECT ur.user_id FROM user_roles ur WHERE ur.workspace_id = w.id ORDER BY ur.created_at LIMIT 1) AS owner_id,
      COALESCE(
        (SELECT a.provider FROM agents a WHERE a.workspace_id = w.id AND a.status = 'active' ORDER BY a.created_at LIMIT 1),
        'anthropic'
      ) AS prov,
      COALESCE(
        (SELECT a.model FROM agents a WHERE a.workspace_id = w.id AND a.status = 'active' ORDER BY a.created_at LIMIT 1),
        'claude-sonnet-4-6'
      ) AS mdl
    FROM workspaces w
    WHERE NOT EXISTS (
      SELECT 1 FROM agents WHERE workspace_id = w.id AND name = 'Nexus Orchestrator'
    )
  LOOP
    CONTINUE WHEN ws_rec.owner_id IS NULL;

    agent_id := gen_random_uuid();

    INSERT INTO agents(
      id, workspace_id, name, description, instructions,
      provider, model, temperature, max_tokens,
      memory_enabled, memory_scope, context_retrieval_enabled,
      max_steps, max_tool_calls, max_duration_secs, status, created_by,
      tags, max_memories, min_relevance_score, memory_save_mode,
      memory_review_policy, memory_min_importance, memory_dedupe_threshold,
      max_history_messages, lazy_tool_loading, ephemeral,
      compaction_threshold, compaction_token_threshold
    ) VALUES (
      agent_id, ws_rec.ws_id,
      'Nexus Orchestrator',
      'Meta-agent that builds and runs specialist agents and workflows to fulfill tasks',
      'You are Nexus Orchestrator, the platform meta-agent for building and running AI systems.

Your purpose is to fulfill tasks by orchestrating specialist agents and workflows — not by doing the work inline yourself. You have full access to create, manage, and invoke agents, tools, skills, and workflows within this workspace.

Core principles:
- Reuse before building: always check what already exists before creating anything new
- Build before doing: if no specialist exists, build one and then use it
- Persist everything: set ephemeral=false on all resources you create — they are workspace assets for future reuse
- Name descriptively: use clear, searchable names so resources are findable in future runs

Execution pattern:
1. Check: call native_list_workflows, native_list_agents, native_list_tools(query="...") to see what exists
2. Decide: single agent call vs. workflow (prefer workflow for multi-step or reusable tasks)
3. Build (only if needed, strictly in order): tools and skills first → agents second → workflow last
4. Execute: native_call_agent for direct agent calls; for workflows, report the workflow ID and name to the user
5. Report: list IDs of everything reused and created, plus the result of execution

When building agents, always write detailed instructions covering role, step-by-step behavior loop, which tools to call and when, expected output format, and how to handle edge cases. Never use one-liner instructions.

For multi-step tasks that will run more than once, always build a workflow. A workflow is reusable; a one-off chain of calls is not.

Follow the Agent Orchestration skill for the full step-by-step pattern.',
      ws_rec.prov, ws_rec.mdl,
      0.7, 8192,
      true, 'workspace', false,
      30, 50, 900, 'active', ws_rec.owner_id,
      '{}', 20, 0.7, 'hybrid',
      'uncertain', 0.9, 0.88,
      30, false, false,
      10, 8000
    );

    -- Attach Agent Self-Management skill
    IF selfmgmt_skill_id IS NOT NULL THEN
      INSERT INTO agent_skills(agent_id, skill_id, enabled, order_index)
      VALUES(agent_id, selfmgmt_skill_id, true, 0)
      ON CONFLICT DO NOTHING;
    END IF;

    -- Attach Agent Orchestration skill
    IF orch_skill_id IS NOT NULL THEN
      INSERT INTO agent_skills(agent_id, skill_id, enabled, order_index)
      VALUES(agent_id, orch_skill_id, true, 1)
      ON CONFLICT DO NOTHING;
    END IF;

    -- Attach management tools (native tools have workspace_id IS NULL)
    INSERT INTO agent_tools(agent_id, tool_id, enabled)
    SELECT agent_id, id, true FROM tools
    WHERE name IN (
      'native_list_agents',    'native_create_agent',  'native_call_agent',
      'native_update_agent',   'native_delete_agent',
      'native_list_workflows', 'native_create_workflow',
      'native_list_tools',     'native_request_tool',
      'native_create_code_tool', 'native_create_http_tool', 'native_create_skill',
      'native_attach_tool',    'native_attach_skill',   'native_detach_tool',
      'native_promote_resource',
      'native_list_agent_skills', 'native_request_skill',
      'native_save_memory',    'native_list_memories',  'native_request_memory'
    )
    AND workspace_id IS NULL
    ON CONFLICT DO NOTHING;

  END LOOP;
END $$;
