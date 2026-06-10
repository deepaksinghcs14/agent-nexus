package domain

import (
	"encoding/json"
	"time"
)

// ============================================================
// USER & WORKSPACE
// ============================================================

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	AvatarURL string    `json:"avatar_url"`
	IsActive  bool      `json:"is_active"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Workspace struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	DisplayName   string          `json:"display_name"`
	OwnerID       string          `json:"owner_id"`
	WorkspaceType string          `json:"workspace_type"`
	Settings      json.RawMessage `json:"settings"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type WorkspaceWithRole struct {
	Workspace
	Role string `json:"role"`
}

// ============================================================
// PROVIDER CREDENTIALS
// ============================================================

type ProviderCredential struct {
	ID               string     `json:"id"`
	WorkspaceID      string     `json:"workspace_id"`
	Provider         string     `json:"provider"` // anthropic | openai | gemini | ollama
	DisplayName      string     `json:"display_name"`
	BaseURL          string     `json:"base_url"` // for ollama / custom
	IsActive         bool       `json:"is_active"`
	AuthType         string     `json:"auth_type"` // api_key | oauth
	OAuthTokenExpiry *time.Time `json:"oauth_token_expiry,omitempty"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	// EncryptedKey and oauth tokens are never returned to clients
}

// ============================================================
// AGENT
// ============================================================

type Agent struct {
	ID                      string    `json:"id"`
	WorkspaceID             string    `json:"workspace_id"`
	Name                    string    `json:"name"`
	Description             string    `json:"description"`
	Instructions            string    `json:"instructions"`
	Provider                string    `json:"provider"`
	Model                   string    `json:"model"`
	Temperature             float64   `json:"temperature"`
	MaxTokens               int       `json:"max_tokens"`
	MemoryEnabled           bool      `json:"memory_enabled"`
	MemoryScope             string    `json:"memory_scope"`
	ContextRetrievalEnabled bool      `json:"context_retrieval_enabled"`
	MaxSteps                int       `json:"max_steps"`
	MaxToolCalls            int       `json:"max_tool_calls"`
	MaxDurationSecs         int       `json:"max_duration_secs"`
	Status                  string    `json:"status"`
	CreatedBy               string    `json:"created_by"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// ============================================================
// TOOL
// ============================================================

type Tool struct {
	ID               string          `json:"id"`
	WorkspaceID      string          `json:"workspace_id,omitempty"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Type             string          `json:"type"` // native | mcp | http
	InputSchema      json.RawMessage `json:"input_schema"`
	OutputSchema     json.RawMessage `json:"output_schema"`
	Config           json.RawMessage `json:"config,omitempty"` // HTTP tool runtime config
	RiskLevel        string          `json:"risk_level"`       // low | medium | high | critical
	RequiresApproval bool            `json:"requires_approval"`
	TimeoutMs        int             `json:"timeout_ms"`
	Enabled          bool            `json:"enabled"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type AgentTool struct {
	AgentID   string          `json:"agent_id"`
	ToolID    string          `json:"tool_id"`
	Enabled   bool            `json:"enabled"`
	Overrides json.RawMessage `json:"overrides"`
	Tool      *Tool           `json:"tool,omitempty"`
}

// ============================================================
// MCP
// ============================================================

type MCPServer struct {
	ID            string          `json:"id"`
	WorkspaceID   string          `json:"workspace_id"`
	Name          string          `json:"name"`
	URL           string          `json:"url"`
	Transport     string          `json:"transport"` // http | stdio
	Status        string          `json:"status"`
	Config        json.RawMessage `json:"config,omitempty"`
	ToolsSyncedAt *time.Time      `json:"tools_synced_at"`
	CreatedBy     string          `json:"created_by"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type MCPTool struct {
	ID          string          `json:"id"`
	ServerID    string          `json:"server_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	RiskLevel   string          `json:"risk_level"`
	Enabled     bool            `json:"enabled"`
}

// ============================================================
// CONVERSATION & MESSAGE
// ============================================================

type Conversation struct {
	ID           string    `json:"id"`
	WorkspaceID  string    `json:"workspace_id"`
	AgentID      string    `json:"agent_id"`
	UserID       string    `json:"user_id"`
	Title        string    `json:"title"`
	MessageCount int       `json:"message_count"`
	TokenCount   int       `json:"token_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Message struct {
	ID             string          `json:"id"`
	ConversationID string          `json:"conversation_id"`
	Role           string          `json:"role"` // user | assistant | tool
	Content        string          `json:"content"`
	ToolCalls      json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID     string          `json:"tool_call_id,omitempty"`
	ToolName       string          `json:"tool_name,omitempty"`
	Tokens         int             `json:"tokens"`
	CreatedAt      time.Time       `json:"created_at"`
}

// ============================================================
// RUN & TRACE
// ============================================================

type RunStatus string

const (
	RunStatusPending      RunStatus = "pending"
	RunStatusRunning      RunStatus = "running"
	RunStatusSuccess      RunStatus = "success"
	RunStatusFailed       RunStatus = "failed"
	RunStatusCancelled    RunStatus = "cancelled"
	RunStatusApprovalWait RunStatus = "approval_wait"
)

type Run struct {
	ID                string     `json:"id"`
	WorkspaceID       string     `json:"workspace_id"`
	AgentID           string     `json:"agent_id"`
	ConversationID    string     `json:"conversation_id"`
	UserID            string     `json:"user_id"`
	Input             string     `json:"input"`
	Output            string     `json:"output"`
	Status            RunStatus  `json:"status"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at"`
	TotalInputTokens  int        `json:"total_input_tokens"`
	TotalOutputTokens int        `json:"total_output_tokens"`
	CostEstimate      float64    `json:"cost_estimate"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	TriggerID         string     `json:"trigger_id,omitempty"`
}

type StepType string

const (
	StepMemoryRetrieval  StepType = "memory_retrieval"
	StepContextRetrieval StepType = "context_retrieval"
	StepModelCall        StepType = "model_call"
	StepToolCall         StepType = "tool_call"
	StepMCPCall          StepType = "mcp_call"
	StepApprovalWait     StepType = "approval_wait"
	StepFinalResponse    StepType = "final_response"
	StepError            StepType = "error"
)

type RunStep struct {
	ID         string          `json:"id"`
	RunID      string          `json:"run_id"`
	StepType   StepType        `json:"step_type"`
	Input      json.RawMessage `json:"input"`
	Output     json.RawMessage `json:"output"`
	LatencyMs  int             `json:"latency_ms"`
	TokensUsed int             `json:"tokens_used"`
	ToolName   string          `json:"tool_name,omitempty"`
	Error      string          `json:"error,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// ============================================================
// MEMORY
// ============================================================

type MemoryScope string

const (
	MemoryScopeConversation MemoryScope = "conversation"
	MemoryScopeAgent        MemoryScope = "agent"
	MemoryScopeWorkspace    MemoryScope = "workspace"
)

type Memory struct {
	ID             string      `json:"id"`
	WorkspaceID    string      `json:"workspace_id"`
	AgentID        string      `json:"agent_id,omitempty"`
	UserID         string      `json:"user_id,omitempty"`
	Scope          MemoryScope `json:"scope"`
	Content        string      `json:"content"`
	RelevanceScore float64     `json:"relevance_score"`
	SourceRunID    string      `json:"source_run_id,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// ============================================================
// CONNECTOR
// ============================================================

type Connector struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	Name        string          `json:"name"`
	Provider    string          `json:"provider"`
	Type        string          `json:"type"`
	AuthType    string          `json:"auth_type"`
	Status      string          `json:"status"`
	Config      json.RawMessage `json:"config,omitempty"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type ConnectorSyncJob struct {
	ID               string     `json:"id"`
	ConnectorID      string     `json:"connector_id"`
	Status           string     `json:"status"`
	StartedAt        *time.Time `json:"started_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	DocumentsFound   int        `json:"documents_found"`
	DocumentsIndexed int        `json:"documents_indexed"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type ConnectorDocument struct {
	ID               string          `json:"id"`
	ConnectorID      string          `json:"connector_id"`
	WorkspaceID      string          `json:"workspace_id"`
	Source           string          `json:"source"`
	SourceDocumentID string          `json:"source_document_id"`
	Title            string          `json:"title"`
	URL              string          `json:"url"`
	Author           string          `json:"author"`
	ContentHash      string          `json:"content_hash"`
	LastModifiedAt   *time.Time      `json:"last_modified_at"`
	IndexedAt        *time.Time      `json:"indexed_at"`
	Metadata         json.RawMessage `json:"metadata"`
}

// ============================================================
// WEBHOOK TRIGGER
// ============================================================

type WebhookTrigger struct {
	ID               string     `json:"id"`
	WorkspaceID      string     `json:"workspace_id"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	TargetType       string     `json:"target_type"` // agent | workflow
	TargetID         string     `json:"target_id"`
	TargetName       string     `json:"target_name,omitempty"`
	InputTemplate    string     `json:"input_template"`
	Secret           string     `json:"secret,omitempty"`
	IsActive         bool       `json:"is_active"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	LastTriggeredAt  *time.Time `json:"last_triggered_at,omitempty"`
	TriggerCount     int64      `json:"trigger_count"`
}

// ============================================================
// ADMIN
// ============================================================

type AuditLog struct {
	ID           string          `json:"id"`
	WorkspaceID  string          `json:"workspace_id,omitempty"`
	ActorID      string          `json:"actor_id,omitempty"`
	ActorEmail   string          `json:"actor_email"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Metadata     json.RawMessage `json:"metadata"`
	IPAddress    string          `json:"ip_address"`
	CreatedAt    time.Time       `json:"created_at"`
}

type Policy struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id,omitempty"`
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}
