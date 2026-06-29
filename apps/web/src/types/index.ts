// src/types/index.ts — mirrors domain/types.go

export interface User {
  id: string
  email: string
  full_name: string
  avatar_url: string
  is_active: boolean
  is_admin: boolean
  created_at: string
  updated_at: string
}

export type WorkspaceType = 'personal' | 'team' | 'organization' | 'project' | 'sandbox'

export interface Workspace {
  id: string
  name: string
  display_name: string
  owner_id: string
  workspace_type: WorkspaceType
  settings: Record<string, unknown>
  created_at: string
  updated_at: string
}

export type WorkspaceRole = 'owner' | 'admin' | 'member' | 'viewer'

export interface WorkspaceWithRole extends Workspace {
  role: WorkspaceRole
}

export interface WorkspaceMember {
  id: string
  email: string
  full_name: string
  avatar_url: string
  is_active: boolean
  role: WorkspaceRole
  joined_at: string
}

export interface ProviderCredential {
  id: string
  workspace_id: string
  provider: 'anthropic' | 'openai' | 'gemini' | 'ollama'
  display_name: string
  base_url: string
  is_active: boolean
  auth_type: 'api_key' | 'oauth'
  oauth_token_expiry?: string
  created_at: string
  updated_at: string
}

export interface ModelInfo {
  id: string
  name: string
  context_window: number
  supports_tools: boolean
  supports_vision: boolean
}

export interface Agent {
  id: string
  workspace_id: string
  name: string
  description: string
  instructions: string
  provider: string
  model: string
  temperature: number
  max_tokens: number
  memory_enabled: boolean
  memory_scope: 'conversation' | 'agent' | 'workspace'
  memory_save_mode: 'tool' | 'extractor' | 'hybrid'
  memory_review_policy: 'none' | 'uncertain' | 'all'
  max_memories: number
  min_relevance_score: number
  memory_min_importance: number
  memory_dedupe_threshold: number
  context_retrieval_enabled: boolean
  agentic_rag: boolean
  max_steps: number
  max_tool_calls: number
  max_duration_secs: number
  max_history_messages: number
  lazy_tool_loading: boolean
  compaction_threshold: number
  compaction_token_threshold: number
  protected: boolean
  status: 'active' | 'paused' | 'archived'
  created_by: string
  created_at: string
  updated_at: string
}

export interface Tool {
  id: string
  workspace_id?: string
  name: string
  description: string
  type: 'native' | 'mcp' | 'http' | 'code'
  input_schema: Record<string, unknown>
  output_schema: Record<string, unknown>
  config?: Record<string, unknown>
  risk_level: 'low' | 'medium' | 'high' | 'critical'
  requires_approval: boolean
  timeout_ms: number
  enabled: boolean
}

export interface MCPServer {
  id: string
  workspace_id: string
  name: string
  url: string
  transport: 'http' | 'stdio'
  status: 'connected' | 'disconnected' | 'error'
  tools_synced_at?: string
  created_at: string
  updated_at: string
}

export interface MCPTool {
  id: string
  server_id: string
  name: string
  description: string
  input_schema: Record<string, unknown>
  risk_level: string
  enabled: boolean
}

export interface Conversation {
  id: string
  workspace_id: string
  agent_id: string
  user_id: string
  title: string
  message_count: number
  token_count: number
  created_at: string
  updated_at: string
}

export interface Message {
  id: string
  conversation_id: string
  role: 'user' | 'assistant' | 'tool'
  content: string
  tool_calls?: ToolCall[]
  tool_call_id?: string
  tool_name?: string
  tokens: number
  created_at: string
}

export interface ToolCall {
  id: string
  name: string
  input: Record<string, unknown>
}

export type RunStatus = 'pending' | 'running' | 'success' | 'failed' | 'cancelled' | 'approval_wait'

export interface Run {
  id: string
  workspace_id: string
  agent_id: string | null
  conversation_id: string
  user_id: string
  input: string
  output: string
  status: RunStatus
  started_at: string
  completed_at?: string
  total_input_tokens: number
  total_output_tokens: number
  cost_estimate: number
  error_message?: string
  trigger_id?: string
  parent_run_id?: string
  workflow_node_id?: string
  trace_id?: string
  channel_session_id?: string
}

export interface ServiceLogEntry {
  ts: string
  source: 'api' | 'whatsapp-adapter' | string
  level: 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal' | string
  message: string
  attrs?: Record<string, unknown>
}

export interface GatewayChannelConfig {
  account_id: string
  adapter_url?: string
  dm_policy?: 'pairing' | 'allowlist' | 'open' | 'disabled'
  allow_from?: string[]
  session_scope?: string
  group_policy?: 'allowlist' | 'open' | 'disabled'
  group_allow_from?: string[]
  groups?: string[]
  history_limit?: number
  self_chat_enabled?: boolean
  assistant_enabled?: boolean
  bot_mode_enabled?: boolean
  chat_approvals_enabled?: boolean
  browser_name?: string
}

export interface GatewayChannel {
  id: string
  workspace_id: string
  agent_id: string
  agent_name?: string
  name: string
  description: string
  channel_type: 'whatsapp' | 'http'
  config: GatewayChannelConfig
  is_active: boolean
  created_by: string
  created_at: string
  updated_at: string
}

export interface ChannelSession {
  id: string
  workspace_id: string
  channel_id: string
  account_id: string
  agent_id: string
  conversation_id: string
  session_key: string
  peer_kind: 'direct' | 'group' | 'channel'
  peer_id: string
  external_sender_id: string
  activation_mode: string
  last_route: Record<string, unknown>
  last_active_at: string
  created_at: string
}

export interface GatewayEvent {
  id: string
  workspace_id: string
  channel_id: string
  session_id?: string
  run_id?: string
  event_type: string
  provider_message_id?: string
  payload: Record<string, unknown>
  created_at: string
}

export interface GatewayPairingRequest {
  id: string
  workspace_id: string
  channel_id: string
  account_id: string
  peer_kind: string
  peer_id: string
  sender_id: string
  code: string
  status: 'pending' | 'approved' | 'rejected' | 'expired'
  expires_at: string
  created_at: string
}

export interface GatewayOutboundMessage {
  id: string
  workspace_id: string
  channel_id: string
  session_id?: string
  run_id?: string
  account_id: string
  peer_kind: string
  peer_id: string
  body: string
  status: 'pending' | 'sent' | 'failed'
  attempts: number
  last_error: string
  created_at: string
  sent_at?: string
}

export interface GatewayContact {
  id: string
  workspace_id: string
  channel_id: string
  account_id: string
  display_name: string
  alias: string
  phone_number: string
  whatsapp_jid: string
  whatsapp_lid?: string
  role: 'owner' | 'trusted' | 'blocked'
  agent_id?: string
  auto_reply_enabled: boolean
  last_matched_at?: string
  created_at: string
  updated_at: string
}

export interface GatewayReminder {
  id: string
  workspace_id: string
  channel_id?: string
  session_id?: string
  contact_id?: string
  use_agent?: boolean
  account_id: string
  title: string
  message: string
  due_at?: string
  status: 'pending' | 'completed' | 'cancelled'
  payload: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface ScheduledMessage {
  id: string
  workspace_id: string
  channel_id: string
  contact_id?: string
  use_agent?: boolean
  account_id: string
  peer_kind: string
  peer_id: string
  message: string
  send_at: string
  status: 'pending' | 'sent' | 'failed' | 'cancelled'
  recurrence_rule?: {
    frequency: 'daily' | 'weekly' | 'monthly' | 'yearly' | 'weekdays'
    interval?: number
    end_at?: string
    max_occurrences?: number
  }
  occurrence_count: number
  last_error?: string
  created_at: string
  updated_at: string
}

export interface GatewayEscalation {
  id: string
  workspace_id: string
  channel_id?: string
  session_id?: string
  run_id?: string
  account_id: string
  action_type: string
  recipient: string
  message: string
  reason: string
  status: 'pending' | 'approved' | 'rejected' | 'resolved'
  approval_code: string
  resolved_by_sender_id?: string
  resolved_at?: string
  payload: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface Skill {
  id: string
  workspace_id?: string
  name: string
  description: string
  content: string
  source: 'manual' | 'managed'
  enabled: boolean
  required_tool_names?: string[]
  created_by?: string
  created_at: string
  updated_at: string
}

export interface AgentSkill {
  id: string
  agent_id: string
  skill_id: string
  enabled: boolean
  activation_mode: string  // 'always' | 'on_demand'
  order_index: number
  created_at: string
  skill?: Skill
}

export interface PaginatedRuns {
  data: Run[]
  has_more: boolean
  next_cursor: string
}

export type StepType =
  | 'memory_retrieval'
  | 'context_retrieval'
  | 'model_call'
  | 'tool_call'
  | 'mcp_call'
  | 'approval_wait'
  | 'final_response'
  | 'error'

export interface RunStep {
  id: string
  run_id: string
  step_type: StepType
  input: Record<string, unknown>
  output: Record<string, unknown>
  latency_ms: number
  tokens_used: number
  tool_name?: string
  error?: string
  created_at: string
}

export interface Memory {
  id: string
  workspace_id: string
  agent_id?: string
  user_id?: string
  conversation_id?: string
  scope: 'conversation' | 'agent' | 'workspace'
  content: string
  relevance_score: number
  importance_score: number
  status: 'active' | 'pending' | 'rejected'
  save_source: 'tool' | 'extractor'
  source_run_id?: string
  metadata?: { reason?: string; [key: string]: unknown }
  created_at: string
  updated_at: string
}

export interface Connector {
  id: string
  workspace_id: string
  name: string
  provider: string
  type: 'native' | 'mcp'
  auth_type: string
  status: 'connected' | 'disconnected' | 'syncing' | 'error'
  config?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface ConnectorSyncJob {
  id: string
  connector_id: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'interrupted'
  started_at?: string
  completed_at?: string
  documents_found: number
  documents_indexed: number
  error_message?: string
  created_at: string
}

export interface ConnectorDocument {
  id: string
  connector_id: string
  workspace_id: string
  source: string
  source_document_id: string
  title: string
  url: string
  author: string
  last_modified_at?: string
  indexed_at?: string
  metadata: Record<string, unknown>
}

export interface ConnectorBrowseDir {
  name: string
  label: string
  path: string
  count?: number
}
export interface ConnectorBrowseFile {
  id: string
  title: string
  url: string
  source_document_id: string
  indexed_at?: string
}
export interface ConnectorBrowseCrumb {
  name: string
  path: string
}
export interface ConnectorBrowseResult {
  path: string
  breadcrumb: ConnectorBrowseCrumb[]
  dirs: ConnectorBrowseDir[]
  files: ConnectorBrowseFile[]
}

export interface ConnectorDocumentGroup {
  key: string
  label: string
  count: number
}

export interface Policy {
  id: string
  workspace_id?: string
  key: string
  value: unknown
  created_at: string
  updated_at: string
}

export interface AuditLog {
  id: string
  workspace_id?: string
  actor_id?: string
  actor_email: string
  action: string
  resource_type: string
  resource_id: string
  metadata: Record<string, unknown>
  ip_address: string
  created_at: string
}

export interface Workflow {
  id: string
  workspace_id: string
  name: string
  description: string
  mode: 'pipeline' | 'supervisor'
  status?: string
  agents?: Agent[]
  agent_ids?: string[]
  run_count?: number
  last_run_at?: string
  created_at: string
  updated_at: string
}

// Backward-compat alias — prefer Workflow in new code
export type AgentGroup = Workflow

// Workflow graph types
export type WorkflowNodeType = 'start' | 'end' | 'agent' | 'supervisor' | 'condition' | 'parallel' | 'join' | 'loop'

export interface WorkflowNode {
  id: string
  node_type: WorkflowNodeType
  agent_id?: string | null
  position_x: number
  position_y: number
  config: Record<string, unknown>
}

export interface WorkflowEdge {
  id: string
  source_node_id: string
  target_node_id: string
  label?: string | null
}

export interface WorkflowGraph {
  nodes: WorkflowNode[]
  edges: WorkflowEdge[]
}

export interface WebhookTrigger {
  id: string
  workspace_id: string
  name: string
  description: string
  target_type: 'agent' | 'workflow'
  target_id: string
  target_name?: string
  input_template: string
  secret?: string
  is_active: boolean
  created_by: string
  created_at: string
  updated_at: string
  last_triggered_at?: string | null
  trigger_count: number
}

// SSE stream event types
export type SSEEventType =
  | 'run_started'
  | 'step_completed'
  | 'delta'
  | 'tool_call'
  | 'approval_required'
  | 'run_completed'
  | 'error'
  | 'node_started'
  | 'node_completed'
  | 'node_routed'

export interface SSEEvent {
  type: SSEEventType
  run_id?: string
  step?: RunStep
  content?: string
  tool?: string
  input?: Record<string, unknown>
  approval_id?: string
  usage?: { input: number; output: number }
  cost?: number
  message?: string
  error?: string
  node_id?: string
  node_name?: string
  node_type?: string
  result?: string
}

// Pagination
export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  per_page: number
  has_more: boolean
}

// API Tokens
export interface APIToken {
  id: string
  name: string
  token_prefix: string
  scopes: string[]
  last_used_at: string | null
  expires_at: string | null
  created_at: string
}

export interface CreatedAPIToken extends APIToken {
  token: string // only present on creation response
}

// Nexus AI chat
export interface NexusMessage {
  id: string
  role: 'user' | 'assistant' | 'tool_event'
  content: string
  toolEvent?: {
    status: 'started' | 'completed' | 'error'
    tool: string
    label?: string
    input?: unknown
    output?: unknown
    result?: { id: string; name: string }
    link?: string
    error?: string
  }
}

// Observability
export interface LatencyByAgent {
  id: string
  name: string
  provider: string
  model: string
  run_count: number
  success_count: number
  p50_secs: number
  p95_secs: number
  avg_input_tokens: number
  avg_output_tokens: number
}

export interface LatencyByModel {
  provider: string
  model: string
  run_count: number
  p50_secs: number
  p95_secs: number
  tokens_per_sec: number
}

export interface LatencyTrendDay {
  day: string
  run_count: number
  p50_secs: number
  p95_secs: number
}

export interface LatencyData {
  by_agent: LatencyByAgent[]
  by_model: LatencyByModel[]
  trend: LatencyTrendDay[]
  days: number
}

// Eval framework
export interface EvalSuite {
  id: string
  workspace_id: string
  agent_id: string
  agent_name?: string
  name: string
  description: string
  grading_mode: 'exact' | 'contains' | 'llm_judge'
  case_count?: number
  last_run_at?: string
  last_score?: number
  created_at: string
  updated_at: string
}

export interface EvalCase {
  id: string
  suite_id: string
  input: string
  expected_output: string
  grading_criteria: string
  created_at: string
  updated_at: string
}

export interface EvalRun {
  id: string
  suite_id: string
  workspace_id: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  total_cases: number
  passed: number
  failed: number
  error_count: number
  score: number
  started_at?: string
  completed_at?: string
  created_at: string
}

export interface EvalResult {
  id: string
  eval_run_id: string
  case_id: string
  run_id?: string
  input?: string
  expected_output?: string
  grading_criteria?: string
  actual_output: string
  passed?: boolean
  score: number
  judge_reasoning: string
  error: string
  latency_ms: number
  created_at: string
}

// Invoke API
export interface InvokeRunResponse {
  run_id: string
  conversation_id: string
  status: 'running' | 'pending'
  workflow_id?: string
  workflow_name?: string
  /** @deprecated use workflow_id */
  group_id?: string
  /** @deprecated use workflow_name */
  group_name?: string
  mode?: string
  message?: string
}
