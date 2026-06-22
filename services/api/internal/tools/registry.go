package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/deepaksingh/agent-nexus/services/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NativeTool is the interface all native tools implement.
type NativeTool interface {
	Definition() domain.Tool
	Execute(input map[string]any) (any, error)
}

type ExecutionContext struct {
	WorkspaceID      string
	AgentID          string
	AgentProvider    string
	AgentModel       string
	UserID           string
	RunID            string
	ConversationID   string
	ChannelSessionID string
	// InvokeDepth is the current agent-call nesting depth (0 = root run).
	InvokeDepth int
	// RootRunID is the trace root run ID for cost/trace attribution.
	RootRunID string
	// CompressText, if non-nil, condenses text via a lightweight LLM call.
	CompressText func(ctx context.Context, text string) (string, error)
	// SearchMemory, if non-nil, returns relevant memories for a query.
	SearchMemory func(ctx context.Context, query string, limit int) ([]domain.Memory, error)
	// RequestMemory, if non-nil, injects selected memories into the current run context.
	RequestMemory func(memories []domain.Memory) bool
	// RequestTool, if non-nil, marks a tool as active for the current run (lazy loading).
	RequestTool func(name string)
	// RequestSkill activates an attached on-demand skill for the current run.
	RequestSkill func(name string) bool
	// ToolSummaries maps tool name → one-line description for the current agent.
	ToolSummaries map[string]string
	// AlwaysActiveTools is the set of meta-tool names always visible in lazy mode.
	// native_list_tools filters these — they can never be "requested".
	AlwaysActiveTools map[string]bool
	// SkillSummaries lists on-demand skills attached to the current agent.
	SkillSummaries map[string]string
	// CallAgent, if non-nil, invokes another workspace agent as a sub-run and returns its output.
	CallAgent func(ctx context.Context, agentID, task string) (string, error)
	// RunWorkflow, if non-nil, triggers a workflow run in the background and returns the run_id.
	RunWorkflow func(ctx context.Context, workflowID, input string) (string, error)
	// SendMessage, if non-nil, sends a mid-run progress message back to the caller channel.
	SendMessage func(ctx context.Context, msg string) error
	// WaitForUserInput, if non-nil, pauses the run and waits for the user to reply.
	WaitForUserInput func(ctx context.Context, question string) (string, error)
}

type ContextAwareTool interface {
	Definition() domain.Tool
	ExecuteWithContext(ctx context.Context, execCtx ExecutionContext, input map[string]any) (any, error)
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]NativeTool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]NativeTool)}
}

func (r *Registry) Register(t NativeTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Definition().Name] = t
}

func (r *Registry) Get(name string) (NativeTool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found in registry", name)
	}
	return t, nil
}

func (r *Registry) All() []domain.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Definition())
	}
	return out
}

// SeedDB upserts all registered native tools into the tools table so they appear
// in the UI and can be assigned to agents. Safe to call on every startup.
func (r *Registry) SeedDB(ctx context.Context, pool *pgxpool.Pool) error {
	for _, t := range r.All() {
		var existingID string
		pool.QueryRow(ctx, `SELECT id::text FROM tools WHERE name=$1 AND workspace_id IS NULL`, t.Name).Scan(&existingID) //nolint:errcheck

		if existingID != "" {
			_, err := pool.Exec(ctx, `
				UPDATE tools SET description=$2, input_schema=$3, risk_level=$4,
				requires_approval=$5, timeout_ms=$6, enabled=true
				WHERE id=$1::uuid`,
				existingID, t.Description, t.InputSchema, t.RiskLevel, t.RequiresApproval, t.TimeoutMs)
			if err != nil {
				return fmt.Errorf("seed tool %q (update): %w", t.Name, err)
			}
		} else {
			outSchema := t.OutputSchema
			if len(outSchema) == 0 {
				outSchema = []byte(`{}`)
			}
			_, err := pool.Exec(ctx, `
				INSERT INTO tools(id, workspace_id, name, description, type, input_schema, output_schema, risk_level, requires_approval, timeout_ms, enabled)
				VALUES ($1::uuid, NULL, $2, $3, 'native', $4, $5, $6, $7, $8, true)`,
				uuid.NewString(), t.Name, t.Description, t.InputSchema, outSchema, t.RiskLevel, t.RequiresApproval, t.TimeoutMs)
			if err != nil {
				return fmt.Errorf("seed tool %q (insert): %w", t.Name, err)
			}
		}
	}
	return nil
}
