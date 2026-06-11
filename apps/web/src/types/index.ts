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
  context_retrieval_enabled: boolean
  max_steps: number
  max_tool_calls: number
  max_duration_secs: number
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
  type: 'native' | 'mcp' | 'http'
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
  scope: 'conversation' | 'agent' | 'workspace'
  content: string
  relevance_score: number
  source_run_id?: string
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
  status: 'pending' | 'running' | 'completed' | 'failed'
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
