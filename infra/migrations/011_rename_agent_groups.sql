-- Rename agent_groups → workflows and agent_group_members → workflow_members
-- for consistent terminology across the platform.

ALTER TABLE agent_groups RENAME TO workflows;
ALTER TABLE agent_group_members RENAME TO workflow_members;

-- Rename the agent_group_id column in conversations → workflow_id
ALTER TABLE conversations RENAME COLUMN agent_group_id TO workflow_id;

-- Rename the index
DROP INDEX IF EXISTS idx_conversations_agent_group_id;
CREATE INDEX IF NOT EXISTS idx_conversations_workflow_id ON conversations(workflow_id);

-- Update FK constraint name for clarity (recreate with new name)
ALTER TABLE workflow_nodes DROP CONSTRAINT IF EXISTS workflow_nodes_workflow_id_fkey;
ALTER TABLE workflow_nodes ADD CONSTRAINT workflow_nodes_workflow_id_fkey
  FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE;

ALTER TABLE workflow_edges DROP CONSTRAINT IF EXISTS workflow_edges_workflow_id_fkey;
ALTER TABLE workflow_edges ADD CONSTRAINT workflow_edges_workflow_id_fkey
  FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE;
