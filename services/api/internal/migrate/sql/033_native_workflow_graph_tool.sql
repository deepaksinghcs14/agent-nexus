-- Migration 033: Native workflow graph self-management
-- Adds native_save_workflow_graph to the Agent Self-Management managed skill.

UPDATE skills
SET required_tool_names = ARRAY(
  SELECT DISTINCT tool_name
  FROM unnest(required_tool_names || ARRAY['native_save_workflow_graph']) AS tool_name
),
content = CASE
  WHEN content LIKE '%native_save_workflow_graph%' THEN content
  ELSE content || E'\n\n**Rich Workflow Graphs**\n- `native_save_workflow_graph(workflow_id, nodes[], edges[])` - replace a workflow canvas with full graph control flow. Use after `native_create_workflow` when you need start/end, condition if/else branches, parallel + join, loop refinement, or supervisor delegation.\n- For rich workflows, call sequence: create/reuse agents -> `native_create_workflow` -> `native_save_workflow_graph` -> `native_run_workflow`.\n- Always include exactly one `start` node and one `end` node. Use `condition` with yes/no edges, `parallel` with two or more branches, `join` after parallel branches, `loop` with `exit_condition` and `max_iterations`, and `supervisor` with `delegate` edges to team agent nodes.\n- `native_run_workflow` waits for completion and returns the final output; do not finish with a background-running placeholder.'
END
WHERE name = 'Agent Self-Management' AND workspace_id IS NULL;
