-- 005_workflow_graph.sql
-- Adds workflow_nodes and workflow_edges tables for the visual workflow engine.
-- Nodes and edges are scoped to an agent_group (workflow_id).

CREATE TABLE workflow_nodes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID NOT NULL REFERENCES agent_groups(id) ON DELETE CASCADE,
    node_type   TEXT NOT NULL,   -- start | agent | condition | parallel | join | loop
    agent_id    UUID REFERENCES agents(id) ON DELETE SET NULL,
    position_x  FLOAT NOT NULL DEFAULT 0,
    position_y  FLOAT NOT NULL DEFAULT 0,
    config      JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE workflow_edges (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id    UUID NOT NULL REFERENCES agent_groups(id) ON DELETE CASCADE,
    source_node_id UUID NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    target_node_id UUID NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    label          TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX ON workflow_nodes(workflow_id);
CREATE INDEX ON workflow_edges(workflow_id);
CREATE INDEX ON workflow_edges(source_node_id);
