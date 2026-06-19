-- AI-created self-management resources should not clutter the workspace by default.

UPDATE skills
SET content = REPLACE(
	REPLACE(
		REPLACE(
			REPLACE(
				content,
				'Set `ephemeral=true` on any resource that should auto-delete when this run ends.',
				'Resources created during a run are temporary by default and auto-delete when the run ends. Set `ephemeral=false` only when the user explicitly asks to keep the resource.'
			),
			'`native_create_workflow(name, mode, agent_ids[])`',
			'`native_create_workflow(name, mode, agent_ids[], ephemeral?)`'
		),
		'`native_run_workflow(workflow_id, input)` - trigger a workflow run in the background, returns run_id',
		'`native_run_workflow(workflow_id, input)` - run a workflow and wait for the final result'
	),
	'For rich workflows, call sequence: create/reuse agents -> `native_create_workflow` -> `native_save_workflow_graph` -> `native_run_workflow`.',
	'For rich workflows, call sequence: create/reuse agents -> `native_create_workflow` -> `native_save_workflow_graph` -> `native_run_workflow`. Temporary agents, skills, tools, and workflows are cleaned up automatically after the run.'
)
WHERE name = 'Agent Self-Management' AND workspace_id IS NULL;
