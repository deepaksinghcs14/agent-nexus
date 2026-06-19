package native

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/deepaksingh/agent-nexus/services/api/internal/tools"
	"github.com/dop251/goja"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── native_create_code_tool ───────────────────────────────────────────────────

type CreateCodeToolTool struct {
	pool *pgxpool.Pool
}

func NewCreateCodeToolTool(pool *pgxpool.Pool) *CreateCodeToolTool {
	return &CreateCodeToolTool{pool: pool}
}

func (t *CreateCodeToolTool) Definition() domain.Tool {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Tool name. Must start with 'code_' prefix (automatically prepended if missing).",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "What this tool does.",
			},
			"code": map[string]any{
				"type":        "string",
				"description": "JS function body. Called as: function(input) { <code> }. Must return a value. No network or filesystem access.",
			},
			"input_schema": map[string]any{
				"type":        "string",
				"description": "JSON string of the input schema. Defaults to {\"type\":\"object\",\"properties\":{}}.",
			},
			"ephemeral": map[string]any{
				"type":        "boolean",
				"description": "If true, tool is deleted when this run ends. Default true; set false only when the user explicitly asks to keep it.",
			},
		},
		"required": []string{"name", "description", "code"},
	})
	return domain.Tool{
		Name:             "native_create_code_tool",
		Description:      "Create a custom JavaScript tool that runs in a sandboxed JS engine. Tools are temporary by default and auto-delete when this run ends unless ephemeral=false is explicitly provided. The code receives `input` and must return a value. No network or filesystem access.",
		Type:             "native",
		InputSchema:      json.RawMessage(schema),
		OutputSchema:     json.RawMessage(`{"type":"object"}`),
		RiskLevel:        "medium",
		RequiresApproval: false,
		TimeoutMs:        10000,
		Enabled:          true,
	}
}

func (t *CreateCodeToolTool) Execute(_ map[string]any) (any, error) {
	return nil, fmt.Errorf("native_create_code_tool requires run context")
}

func (t *CreateCodeToolTool) ExecuteWithContext(ctx context.Context, execCtx tools.ExecutionContext, input map[string]any) (any, error) {
	name, _ := input["name"].(string)
	description, _ := input["description"].(string)
	code, _ := input["code"].(string)
	inputSchemaStr, _ := input["input_schema"].(string)
	ephemeral := ephemeralFromInput(input)

	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if description == "" {
		return nil, fmt.Errorf("description is required")
	}
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}

	// Enforce "code_" prefix
	if !strings.HasPrefix(name, "code_") {
		name = "code_" + name
	}

	// Parse or default input_schema
	var inputSchemaJSON json.RawMessage
	if inputSchemaStr != "" {
		var parsed any
		if err := json.Unmarshal([]byte(inputSchemaStr), &parsed); err != nil {
			return nil, fmt.Errorf("input_schema is not valid JSON: %w", err)
		}
		inputSchemaJSON = json.RawMessage(inputSchemaStr)
	} else {
		inputSchemaJSON = json.RawMessage(`{"type":"object","properties":{}}`)
	}

	configJSON, _ := json.Marshal(map[string]any{
		"language": "javascript",
		"code":     code,
	})

	toolID := uuid.NewString()

	var sourceRunID *string
	if execCtx.RunID != "" {
		sourceRunID = &execCtx.RunID
	}

	_, err := t.pool.Exec(ctx, `
		INSERT INTO tools(id, workspace_id, name, description, type, input_schema, output_schema, config, risk_level, requires_approval, timeout_ms, enabled, source_run_id, ephemeral)
		VALUES($1::uuid, $2::uuid, $3, $4, 'code', $5::jsonb, '{"type":"object"}'::jsonb, $6::jsonb, 'medium', false, 30000, true, $7, $8)`,
		toolID,
		execCtx.WorkspaceID,
		name,
		description,
		json.RawMessage(inputSchemaJSON),
		json.RawMessage(configJSON),
		sourceRunID,
		ephemeral,
	)
	if err != nil {
		// If the tool already exists (unique constraint), look it up and attach it.
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			var existingID string
			lookupErr := t.pool.QueryRow(ctx,
				`SELECT id::text FROM tools WHERE workspace_id=$1::uuid AND name=$2`,
				execCtx.WorkspaceID, name).Scan(&existingID)
			if lookupErr != nil {
				return nil, fmt.Errorf("create code tool: tool already exists but could not retrieve it: %w", lookupErr)
			}
			if execCtx.AgentID != "" {
				t.pool.Exec(ctx, //nolint:errcheck
					`INSERT INTO agent_tools(agent_id, tool_id, enabled)
					 VALUES($1::uuid, $2::uuid, true) ON CONFLICT DO NOTHING`,
					execCtx.AgentID, existingID)
			}
			return map[string]any{
				"tool_id":   existingID,
				"name":      name,
				"ephemeral": ephemeral,
				"note":      "Tool already existed and has been attached. Use the exact name above to call it.",
			}, nil
		}
		return nil, fmt.Errorf("create code tool: %w", err)
	}

	// Auto-attach to calling agent so it's immediately usable
	if execCtx.AgentID != "" {
		t.pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO agent_tools(agent_id, tool_id, enabled)
			 VALUES($1::uuid, $2::uuid, true) ON CONFLICT DO NOTHING`,
			execCtx.AgentID, toolID)
	}

	return map[string]any{
		"tool_id":   toolID,
		"name":      name,
		"ephemeral": ephemeral,
		"note":      "Tool created and attached. The exact tool name (with 'code_' prefix) is in the 'name' field above — use that exact name when calling it. It is callable immediately in your next message.",
	}, nil
}

// ── ExecuteCodeTool ───────────────────────────────────────────────────────────

// ExecuteCodeTool runs JS code in a sandboxed goja VM.
// The code is wrapped as: (function(input){ <code> })(input)
// rawInput is parsed and passed as the `input` variable.
func ExecuteCodeTool(ctx context.Context, code string, rawInput json.RawMessage) (any, error) {
	// Parse input into a plain Go map so goja can consume it
	var inputMap map[string]any
	if len(rawInput) > 0 {
		if err := json.Unmarshal(rawInput, &inputMap); err != nil {
			return nil, fmt.Errorf("code tool: parse input: %w", err)
		}
	}
	if inputMap == nil {
		inputMap = map[string]any{}
	}

	vm := goja.New()

	// Set up timeout via interrupt
	timer := time.AfterFunc(5*time.Second, func() {
		vm.Interrupt("timeout")
	})
	defer timer.Stop()

	if err := vm.Set("input", inputMap); err != nil {
		return nil, fmt.Errorf("code tool: set input: %w", err)
	}

	// Normalise code: if the LLM wrote "function(input){...}" or
	// "function name(input){...}" treat it as an expression and call it directly.
	trimmed := strings.TrimSpace(code)
	var src string
	if strings.HasPrefix(trimmed, "function") {
		src = "(" + trimmed + ")(input)"
	} else {
		src = "(function(input){ " + code + " })(input)"
	}

	val, err := vm.RunString(src)
	if err != nil {
		return nil, fmt.Errorf("code tool: execution error: %w", err)
	}

	return val.Export(), nil
}
