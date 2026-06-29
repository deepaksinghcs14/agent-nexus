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
	ID                       string    `json:"id"`
	WorkspaceID              string    `json:"workspace_id"`
	Name                     string    `json:"name"`
	Description              string    `json:"description"`
	Instructions             string    `json:"instructions"`
	Provider                 string    `json:"provider"`
	Model                    string    `json:"model"`
	Temperature              float64   `json:"temperature"`
	MaxTokens                int       `json:"max_tokens"`
	MemoryEnabled            bool      `json:"memory_enabled"`
	MemoryScope              string    `json:"memory_scope"`
	MemorySaveMode           string    `json:"memory_save_mode"`
	MemoryReviewPolicy       string    `json:"memory_review_policy"`
	MaxMemories              int       `json:"max_memories"`
	MinRelevanceScore        float64   `json:"min_relevance_score"`
	MemoryMinImportance      float64   `json:"memory_min_importance"`
	MemoryDedupeThreshold    float64   `json:"memory_dedupe_threshold"`
	ContextRetrievalEnabled  bool      `json:"context_retrieval_enabled"`
	AgenticRAG               bool      `json:"agentic_rag"`
	MaxSteps                 int       `json:"max_steps"`
	MaxToolCalls             int       `json:"max_tool_calls"`
	MaxDurationSecs          int       `json:"max_duration_secs"`
	MaxHistoryMessages       int       `json:"max_history_messages"` // 0 = default (20)
	LazyToolLoading          bool      `json:"lazy_tool_loading"`
	CompactionThreshold      int       `json:"compaction_threshold"`       // 0 = default (6)
	CompactionTokenThreshold int       `json:"compaction_token_threshold"` // 0 = default (3000)
	Protected                bool      `json:"protected"`
	Status                   string    `json:"status"`
	CreatedBy                string    `json:"created_by"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
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
	Compaction   string    `json:"compaction,omitempty"`
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
	ParentRunID       string     `json:"parent_run_id,omitempty"`
	WorkflowNodeID    string     `json:"workflow_node_id,omitempty"`
	TraceID           string     `json:"trace_id,omitempty"`
	ChannelSessionID  string     `json:"channel_session_id,omitempty"`
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
// GATEWAY
// ============================================================

type GatewayChannel struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	AgentID     string          `json:"agent_id"`
	AgentName   string          `json:"agent_name,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	ChannelType string          `json:"channel_type"`
	Config      json.RawMessage `json:"config"`
	IsActive    bool            `json:"is_active"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type GatewayChannelConfig struct {
	AccountID            string   `json:"account_id"`
	AdapterURL           string   `json:"adapter_url,omitempty"`
	DMPolicy             string   `json:"dm_policy,omitempty"`
	AllowFrom            []string `json:"allow_from,omitempty"`
	SessionScope         string   `json:"session_scope,omitempty"`
	GroupPolicy          string   `json:"group_policy,omitempty"`
	GroupAllowFrom       []string `json:"group_allow_from,omitempty"`
	Groups               []string `json:"groups,omitempty"`
	HistoryLimit         int      `json:"history_limit,omitempty"`
	SelfChatEnabled      bool     `json:"self_chat_enabled,omitempty"`
	AssistantEnabled     bool     `json:"assistant_enabled,omitempty"`
	BotModeEnabled       bool     `json:"bot_mode_enabled,omitempty"`
	ChatApprovalsEnabled bool     `json:"chat_approvals_enabled,omitempty"`
	BrowserName          string   `json:"browser_name,omitempty"`
}

type GatewayChannelAccount struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	ChannelID   string     `json:"channel_id"`
	AccountID   string     `json:"account_id"`
	Status      string     `json:"status"`
	SelfID      string     `json:"self_id"`
	LastError   string     `json:"last_error"`
	LastSeenAt  *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type ChannelSession struct {
	ID               string          `json:"id"`
	WorkspaceID      string          `json:"workspace_id"`
	ChannelID        string          `json:"channel_id"`
	AccountID        string          `json:"account_id"`
	AgentID          string          `json:"agent_id"`
	ConversationID   string          `json:"conversation_id"`
	SessionKey       string          `json:"session_key"`
	PeerKind         string          `json:"peer_kind"`
	PeerID           string          `json:"peer_id"`
	ExternalSenderID string          `json:"external_sender_id"`
	ActivationMode   string          `json:"activation_mode"`
	LastRoute        json.RawMessage `json:"last_route"`
	LastActiveAt     time.Time       `json:"last_active_at"`
	CreatedAt        time.Time       `json:"created_at"`
}

type GatewayEvent struct {
	ID                string          `json:"id"`
	WorkspaceID       string          `json:"workspace_id"`
	ChannelID         string          `json:"channel_id"`
	SessionID         string          `json:"session_id,omitempty"`
	RunID             string          `json:"run_id,omitempty"`
	EventType         string          `json:"event_type"`
	ProviderMessageID string          `json:"provider_message_id,omitempty"`
	Payload           json.RawMessage `json:"payload"`
	CreatedAt         time.Time       `json:"created_at"`
}

type GatewayPairingRequest struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	ChannelID   string    `json:"channel_id"`
	AccountID   string    `json:"account_id"`
	PeerKind    string    `json:"peer_kind"`
	PeerID      string    `json:"peer_id"`
	SenderID    string    `json:"sender_id"`
	Code        string    `json:"code"`
	Status      string    `json:"status"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type GatewayOutboundMessage struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	ChannelID   string     `json:"channel_id"`
	SessionID   string     `json:"session_id,omitempty"`
	RunID       string     `json:"run_id,omitempty"`
	AccountID   string     `json:"account_id"`
	PeerKind    string     `json:"peer_kind"`
	PeerID      string     `json:"peer_id"`
	Body        string     `json:"body"`
	Source      string     `json:"source,omitempty"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	LastError   string     `json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
}

type GatewayContact struct {
	ID               string     `json:"id"`
	WorkspaceID      string     `json:"workspace_id"`
	ChannelID        string     `json:"channel_id"`
	AccountID        string     `json:"account_id"`
	DisplayName      string     `json:"display_name"`
	Alias            string     `json:"alias"`
	PhoneNumber      string     `json:"phone_number"`
	WhatsAppJID      string     `json:"whatsapp_jid"`
	WhatsAppLID      string     `json:"whatsapp_lid,omitempty"`
	Role             string     `json:"role"`
	AgentID          string     `json:"agent_id,omitempty"`
	AutoReplyEnabled bool       `json:"auto_reply_enabled"`
	LastMatchedAt    *time.Time `json:"last_matched_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type GatewayReminder struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	ChannelID   string          `json:"channel_id,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	ContactID   string          `json:"contact_id,omitempty"`
	UseAgent    bool            `json:"use_agent,omitempty"`
	AccountID   string          `json:"account_id"`
	Title       string          `json:"title"`
	Message     string          `json:"message"`
	DueAt       *time.Time      `json:"due_at,omitempty"`
	Status      string          `json:"status"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type ScheduledMessage struct {
	ID              string          `json:"id"`
	WorkspaceID     string          `json:"workspace_id"`
	ChannelID       string          `json:"channel_id"`
	ContactID       string          `json:"contact_id,omitempty"`
	UseAgent        bool            `json:"use_agent,omitempty"`
	AccountID       string          `json:"account_id"`
	PeerKind        string          `json:"peer_kind"`
	PeerID          string          `json:"peer_id"`
	Message         string          `json:"message"`
	SendAt          time.Time       `json:"send_at"`
	Status          string          `json:"status"`
	RecurrenceRule  json.RawMessage `json:"recurrence_rule,omitempty"`
	OccurrenceCount int             `json:"occurrence_count"`
	LastError       string          `json:"last_error,omitempty"`
	CreatedBy       string          `json:"created_by,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type GatewayEscalation struct {
	ID                 string          `json:"id"`
	WorkspaceID        string          `json:"workspace_id"`
	ChannelID          string          `json:"channel_id,omitempty"`
	SessionID          string          `json:"session_id,omitempty"`
	RunID              string          `json:"run_id,omitempty"`
	AccountID          string          `json:"account_id"`
	ActionType         string          `json:"action_type"`
	Recipient          string          `json:"recipient"`
	Message            string          `json:"message"`
	Reason             string          `json:"reason"`
	Status             string          `json:"status"`
	ApprovalCode       string          `json:"approval_code"`
	ResolvedBySenderID string          `json:"resolved_by_sender_id,omitempty"`
	ResolvedAt         *time.Time      `json:"resolved_at,omitempty"`
	Payload            json.RawMessage `json:"payload"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// ============================================================
// SKILLS
// ============================================================

type Skill struct {
	ID                string    `json:"id"`
	WorkspaceID       string    `json:"workspace_id,omitempty"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Content           string    `json:"content"`
	Source            string    `json:"source"`
	Enabled           bool      `json:"enabled"`
	RequiredToolNames []string  `json:"required_tool_names,omitempty"`
	CreatedBy         string    `json:"created_by,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type AgentSkill struct {
	ID             string    `json:"id"`
	AgentID        string    `json:"agent_id"`
	SkillID        string    `json:"skill_id"`
	Enabled        bool      `json:"enabled"`
	ActivationMode string    `json:"activation_mode"`
	OrderIndex     int       `json:"order_index"`
	CreatedAt      time.Time `json:"created_at"`
	Skill          *Skill    `json:"skill,omitempty"`
}

type AgentSkillAssignment struct {
	SkillID        string `json:"skill_id"`
	Enabled        bool   `json:"enabled"`
	ActivationMode string `json:"activation_mode,omitempty"`
	OrderIndex     int    `json:"order_index"`
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
	ID              string          `json:"id"`
	WorkspaceID     string          `json:"workspace_id"`
	AgentID         string          `json:"agent_id,omitempty"`
	UserID          string          `json:"user_id,omitempty"`
	ConversationID  string          `json:"conversation_id,omitempty"`
	Scope           MemoryScope     `json:"scope"`
	Content         string          `json:"content"`
	RelevanceScore  float64         `json:"relevance_score"`
	ImportanceScore float64         `json:"importance_score"`
	Status          string          `json:"status"`
	SaveSource      string          `json:"save_source"`
	SourceRunID     string          `json:"source_run_id,omitempty"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
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
	ID              string     `json:"id"`
	WorkspaceID     string     `json:"workspace_id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	TargetType      string     `json:"target_type"` // agent | workflow
	TargetID        string     `json:"target_id"`
	TargetName      string     `json:"target_name,omitempty"`
	InputTemplate   string     `json:"input_template"`
	Secret          string     `json:"secret,omitempty"`
	IsActive        bool       `json:"is_active"`
	CreatedBy       string     `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	TriggerCount    int64      `json:"trigger_count"`
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

type EvalSuite struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	AgentID     string     `json:"agent_id"`
	AgentName   string     `json:"agent_name,omitempty"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	GradingMode string     `json:"grading_mode"`
	AutoRun     bool       `json:"auto_run"`
	CaseCount   int        `json:"case_count,omitempty"`
	LastRunAt   *time.Time `json:"last_run_at,omitempty"`
	LastScore   *float64   `json:"last_score,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type EvalCase struct {
	ID              string    `json:"id"`
	SuiteID         string    `json:"suite_id"`
	Input           string    `json:"input"`
	ExpectedOutput  string    `json:"expected_output"`
	GradingCriteria string    `json:"grading_criteria"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type EvalRun struct {
	ID          string     `json:"id"`
	SuiteID     string     `json:"suite_id"`
	WorkspaceID string     `json:"workspace_id"`
	Status      string     `json:"status"`
	TotalCases  int        `json:"total_cases"`
	Passed      int        `json:"passed"`
	Failed      int        `json:"failed"`
	ErrorCount  int        `json:"error_count"`
	Score       float64    `json:"score"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type EvalResult struct {
	ID              string    `json:"id"`
	EvalRunID       string    `json:"eval_run_id"`
	CaseID          string    `json:"case_id"`
	RunID           *string   `json:"run_id"`
	Input           string    `json:"input,omitempty"`
	ExpectedOutput  string    `json:"expected_output,omitempty"`
	GradingCriteria string    `json:"grading_criteria,omitempty"`
	ActualOutput    string    `json:"actual_output"`
	Passed          *bool     `json:"passed"`
	Score           float64   `json:"score"`
	JudgeReasoning  string    `json:"judge_reasoning"`
	Error           string    `json:"error"`
	LatencyMs       int       `json:"latency_ms"`
	CreatedAt       time.Time `json:"created_at"`
}
