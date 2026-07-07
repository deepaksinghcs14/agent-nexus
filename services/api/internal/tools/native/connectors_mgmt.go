package native

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

// resolveAgentAndConnector resolves the target agent (defaults to the calling
// agent) and the connector by id-or-name within the workspace. Shared by the
// attach and detach tools.
func resolveAgentAndConnector(ctx context.Context, pool *pgxpool.Pool, execCtx tools.ExecutionContext, input map[string]any) (agentID, connectorID, connectorName string, err error) {
	agentID, _ = input["agent_id"].(string)
	if agentID == "" {
		agentID = execCtx.AgentID
	}
	if agentID == "" {
		return "", "", "", fmt.Errorf("agent_id is required")
	}

	if cid, _ := input["connector_id"].(string); cid != "" {
		err = pool.QueryRow(ctx,
			`SELECT id::text, name FROM connectors WHERE id=$1::uuid AND workspace_id=$2::uuid`,
			cid, execCtx.WorkspaceID).Scan(&connectorID, &connectorName)
	} else if cname, _ := input["connector_name"].(string); cname != "" {
		err = pool.QueryRow(ctx,
			`SELECT id::text, name FROM connectors WHERE name=$1 AND workspace_id=$2::uuid LIMIT 1`,
			cname, execCtx.WorkspaceID).Scan(&connectorID, &connectorName)
	} else {
		return "", "", "", fmt.Errorf("connector_id or connector_name is required")
	}
	if err != nil {
		return "", "", "", fmt.Errorf("connector not found")
	}
	return agentID, connectorID, connectorName, nil
}

// ── native_attach_connector ───────────────────────────────────────────────────

type AttachConnectorTool struct{ pool *pgxpool.Pool }

func NewAttachConnectorTool(pool *pgxpool.Pool) *AttachConnectorTool {
	return &AttachConnectorTool{pool}
}

func (t *AttachConnectorTool) Definition() domain.Tool {
	schema := json.RawMessage(`{"type":"object","properties":{
		"agent_id":{"type":"string","description":"UUID of the agent. Defaults to the calling agent."},
		"connector_id":{"type":"string","description":"UUID of the connector to attach."},
		"connector_name":{"type":"string","description":"Name of the connector (alternative to connector_id)."}
	}}`)
	return domain.Tool{
		Name:             "native_attach_connector",
		Description:      "Attach a data-source connector to an agent so its knowledge can be retrieved as context. Enables context retrieval on the agent. Pass connector_id or connector_name; agent_id defaults to the calling agent.",
		Type:             "native",
		InputSchema:      schema,
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}

func (t *AttachConnectorTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_attach_connector requires run context")
}

func (t *AttachConnectorTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	agentID, connectorID, connectorName, err := resolveAgentAndConnector(ctx, t.pool, execCtx, input)
	if err != nil {
		return nil, err
	}

	if _, err := t.pool.Exec(ctx,
		`INSERT INTO agent_connectors(agent_id, connector_id, enabled)
		 VALUES($1::uuid, $2::uuid, true)
		 ON CONFLICT(agent_id, connector_id) DO UPDATE SET enabled=true`,
		agentID, connectorID); err != nil {
		return nil, fmt.Errorf("attach connector: %w", err)
	}
	// A connector is only used when context retrieval is on — enable it.
	t.pool.Exec(ctx, //nolint:errcheck
		`UPDATE agents SET context_retrieval_enabled=true, updated_at=NOW()
		 WHERE id=$1::uuid AND context_retrieval_enabled=false`,
		agentID)

	return map[string]any{
		"attached":       true,
		"agent_id":       agentID,
		"connector_id":   connectorID,
		"connector_name": connectorName,
	}, nil
}

// ── native_detach_connector ───────────────────────────────────────────────────

type DetachConnectorTool struct{ pool *pgxpool.Pool }

func NewDetachConnectorTool(pool *pgxpool.Pool) *DetachConnectorTool {
	return &DetachConnectorTool{pool}
}

func (t *DetachConnectorTool) Definition() domain.Tool {
	schema := json.RawMessage(`{"type":"object","properties":{
		"agent_id":{"type":"string","description":"UUID of the agent. Defaults to the calling agent."},
		"connector_id":{"type":"string","description":"UUID of the connector to detach."},
		"connector_name":{"type":"string","description":"Name of the connector (alternative to connector_id)."}
	}}`)
	return domain.Tool{
		Name:             "native_detach_connector",
		Description:      "Detach a data-source connector from an agent so its knowledge is no longer retrieved as context. Pass connector_id or connector_name; agent_id defaults to the calling agent.",
		Type:             "native",
		InputSchema:      schema,
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        5000,
		Enabled:          true,
	}
}

func (t *DetachConnectorTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_detach_connector requires run context")
}

func (t *DetachConnectorTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	agentID, connectorID, connectorName, err := resolveAgentAndConnector(ctx, t.pool, execCtx, input)
	if err != nil {
		return nil, err
	}

	if _, err := t.pool.Exec(ctx,
		`DELETE FROM agent_connectors WHERE agent_id=$1::uuid AND connector_id=$2::uuid`,
		agentID, connectorID); err != nil {
		return nil, fmt.Errorf("detach connector: %w", err)
	}

	return map[string]any{
		"detached":       true,
		"agent_id":       agentID,
		"connector_id":   connectorID,
		"connector_name": connectorName,
	}, nil
}
