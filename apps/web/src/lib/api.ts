// src/lib/api.ts — all API calls go through this wrapper
import type { WorkspaceWithRole, Workspace, APIToken, CreatedAPIToken, InvokeRunResponse, WorkflowGraph } from '@/types'

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

class APIClient {
  private baseURL: string

  constructor(baseURL: string) {
    this.baseURL = baseURL
  }

  private getToken(): string | null {
    if (typeof window === 'undefined') return null
    return localStorage.getItem('access_token')
  }

  private headers(): HeadersInit {
    const h: HeadersInit = { 'Content-Type': 'application/json' }
    const token = this.getToken()
    if (token) h['Authorization'] = `Bearer ${token}`
    return h
  }

  async get<T>(path: string): Promise<T> {
    const res = await fetch(`${this.baseURL}/api/v1${path}`, {
      headers: this.headers(),
      credentials: 'include',
    })
    return this.handle<T>(res)
  }

  async post<T>(path: string, body?: unknown): Promise<T> {
    const res = await fetch(`${this.baseURL}/api/v1${path}`, {
      method: 'POST',
      headers: this.headers(),
      body: body ? JSON.stringify(body) : undefined,
      credentials: 'include',
    })
    return this.handle<T>(res)
  }

  async put<T>(path: string, body?: unknown): Promise<T> {
    const res = await fetch(`${this.baseURL}/api/v1${path}`, {
      method: 'PUT',
      headers: this.headers(),
      body: body ? JSON.stringify(body) : undefined,
      credentials: 'include',
    })
    return this.handle<T>(res)
  }

  async patch<T>(path: string, body?: unknown): Promise<T> {
    const res = await fetch(`${this.baseURL}/api/v1${path}`, {
      method: 'PATCH',
      headers: this.headers(),
      body: body ? JSON.stringify(body) : undefined,
      credentials: 'include',
    })
    return this.handle<T>(res)
  }

  async delete<T>(path: string): Promise<T> {
    const res = await fetch(`${this.baseURL}/api/v1${path}`, {
      method: 'DELETE',
      headers: this.headers(),
      credentials: 'include',
    })
    return this.handle<T>(res)
  }

  // Returns a native EventSource for SSE streams.
  sse(path: string): EventSource {
    const token = this.getToken()
    const url = new URL(`${this.baseURL}/api/v1${path}`)
    if (token) url.searchParams.set('token', token)
    return new EventSource(url.toString())
  }

  private async handle<T>(res: Response): Promise<T> {
    if (res.status === 401) {
      // Try token refresh
      const refreshed = await this.refreshToken()
      if (!refreshed) {
        window.location.href = '/login'
        throw new Error('Unauthorized')
      }
      // Retry once (caller should retry via React Query)
      throw new Error('Token refreshed — retry request')
    }

    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }))
      throw new Error(err.error ?? err.message ?? 'Unknown error')
    }

    if (res.status === 204) return undefined as T
    return res.json()
  }

  private async refreshToken(): Promise<boolean> {
    try {
      const res = await fetch(`${this.baseURL}/api/v1/auth/refresh`, {
        method: 'POST',
        credentials: 'include', // refresh token in httpOnly cookie
      })
      if (!res.ok) return false
      const data = await res.json()
      localStorage.setItem('access_token', data.access_token)
      return true
    } catch {
      return false
    }
  }
}

export const api = new APIClient(API_URL)

// Convenience wrappers used by React Query hooks
export const agentsAPI = {
  list: () => api.get('/agents'),
  get: (id: string) => api.get(`/agents/${id}`),
  create: (body: unknown) => api.post('/agents', body),
  update: (id: string, body: unknown) => api.put(`/agents/${id}`, body),
  delete: (id: string) => api.delete(`/agents/${id}`),
  getTools: (id: string) => api.get(`/agents/${id}/tools`),
  setTools: (id: string, body: unknown) => api.put(`/agents/${id}/tools`, body),
  getConnectors: (id: string) => api.get(`/agents/${id}/connectors`),
  setConnectors: (id: string, body: { connector_ids: string[]; max_chunks: number; min_score: number }) =>
    api.put(`/agents/${id}/connectors`, body),
  getSkills: (id: string) => api.get(`/agents/${id}/skills`),
  setSkills: (id: string, body: unknown) => api.put(`/agents/${id}/skills`, body),
  exportUrl: (id: string) => `${API_URL}/api/v1/agents/${id}/export`,
  importAgent: (body: unknown) => api.post('/agents/import', body),
}

export const runsAPI = {
  stats: () => api.get('/runs/stats'),
  list: (params?: string | Record<string, string>) => {
    if (typeof params === 'object') {
      const qs = new URLSearchParams(params).toString()
      return api.get(`/runs${qs ? '?' + qs : ''}`)
    }
    return api.get(`/runs${params ? '?' + params : ''}`)
  },
  listPage: (params: Record<string, string> = {}) => {
    const qs = new URLSearchParams(params).toString()
    return api.get(`/runs${qs ? '?' + qs : ''}`)
  },
  get: (id: string) => api.get(`/runs/${id}`),
  listChildren: (id: string) => api.get(`/runs/${id}/children`),
  start: (conversationId: string, body: unknown) =>
    api.post(`/conversations/${conversationId}/runs`, body),
  approve: (id: string, body: unknown) => api.post(`/runs/${id}/approve`, body),
  submitInput: (id: string, body: { answer: string }) => api.post(`/runs/${id}/input`, body),
  cancel: (id: string) => api.post(`/runs/${id}/cancel`),
}

export const memoryAPI = {
  list: (params?: string) => api.get(`/memory${params ? '?' + params : ''}`),
  approve: (id: string) => api.patch(`/memory/${id}/approve`),
  reject: (id: string) => api.patch(`/memory/${id}/reject`),
  delete: (id: string) => api.delete(`/memory/${id}`),
  bulkDelete: (params?: string) => api.delete(`/memory${params ? '?' + params : ''}`),
}

export const toolsAPI = {
  list: () => api.get('/tools'),
  create: (body: unknown) => api.post('/tools', body),
  update: (id: string, body: unknown) => api.put(`/tools/${id}`, body),
  delete: (id: string) => api.delete(`/tools/${id}`),
}

export const connectorsAPI = {
  list: () => api.get('/connectors'),
  create: (body: unknown) => api.post('/connectors', body),
  sync: (id: string) => api.post(`/connectors/${id}/sync`),
  documents: (id: string, page = 1, group?: string) => api.get(`/connectors/${id}/documents?page=${page}${group ? `&group=${encodeURIComponent(group)}` : ''}`),
  documentGroups: (id: string) => api.get(`/connectors/${id}/document-groups`),
  browse: (id: string, path = '', search = '') => api.get(`/connectors/${id}/browse?path=${encodeURIComponent(path)}${search ? `&search=${encodeURIComponent(search)}` : ''}`),
  syncJobs: (id: string) => api.get(`/connectors/${id}/sync-jobs`),
  delete: (id: string) => api.delete(`/connectors/${id}`),
}

export const filesystemAPI = {
  browse: (path: string) =>
    api.get<{ path: string; parent: string; entries: { name: string; path: string; is_dir: boolean }[] }>(
      `/filesystem/browse?path=${encodeURIComponent(path)}`
    ),
}

export const mcpAPI = {
  list: () => api.get('/mcp-servers'),
  create: (body: unknown) => api.post('/mcp-servers', body),
  sync: (id: string) => api.post(`/mcp-servers/${id}/sync`),
  tools: (id: string) => api.get(`/mcp-servers/${id}/tools`),
  updateToolRisk: (serverId: string, toolId: string, riskLevel: string) =>
    api.patch(`/mcp-servers/${serverId}/tools/${toolId}`, { risk_level: riskLevel }),
  delete: (id: string) => api.delete(`/mcp-servers/${id}`),
}

export const authAPI = {
  login: (body: { email: string; password: string }) =>
    api.post<{ access_token: string; user: Record<string, unknown>; workspace_id: string }>('/auth/login', body),
  register: (body: { email: string; password: string; full_name: string }) =>
    api.post<{ access_token: string; user: Record<string, unknown>; workspace_id: string }>('/auth/register', body),
  logout: () => api.post('/auth/logout'),
  me: () => api.get<{ user: Record<string, unknown>; workspace: Record<string, unknown> }>('/auth/me'),
  refresh: () => api.post<{ access_token: string }>('/auth/refresh'),
}

export const providersAPI = {
  list: () => api.get('/providers'),
  create: (body: unknown) => api.post('/providers', body),
  update: (id: string, body: unknown) => api.put(`/providers/${id}`, body),
  delete: (id: string) => api.delete(`/providers/${id}`),
  models: (id: string) => api.get(`/providers/${id}/models`),
}

export const conversationsAPI = {
  list: () => api.get('/conversations'),
  create: (body: { agent_id: string; title?: string }) => api.post('/conversations', body),
  get: (id: string) => api.get(`/conversations/${id}`),
  delete: (id: string) => api.delete(`/conversations/${id}`),
  deleteAll: () => api.delete('/conversations'),
}

export const workspaceAPI = {
  get: () => api.get('/workspace'),
  update: (body: { display_name: string; workspace_type?: string }) => api.patch('/workspace', body),
  members: () => api.get('/workspace/members'),
  addMember: (body: unknown) => api.post('/workspace/members', body),
  updateMember: (id: string, body: unknown) => api.patch(`/workspace/members/${id}`, body),
  removeMember: (id: string) => api.delete(`/workspace/members/${id}`),
}

export const workspacesAPI = {
  list: () => api.get<{ data: WorkspaceWithRole[] }>('/workspaces'),
  create: (body: { display_name: string; workspace_type: string }) =>
    api.post<Workspace>('/workspaces', body),
  switch: (workspace_id: string) =>
    api.post<{ access_token: string; workspace_id: string }>('/workspaces/switch', { workspace_id }),
  delete: (id: string) => api.delete<void>(`/workspaces/${id}`),
}

export const workflowsAPI = {
  list: () => api.get('/workflows'),
  create: (body: unknown) => api.post('/workflows', body),
  get: (id: string) => api.get(`/workflows/${id}`),
  update: (id: string, body: unknown) => api.put(`/workflows/${id}`, body),
  delete: (id: string) => api.delete(`/workflows/${id}`),
  run: (id: string, body?: unknown) => api.post(`/workflows/${id}/runs`, body),
  getGraph: (id: string) => api.get<WorkflowGraph>(`/workflows/${id}/graph`),
  saveGraph: (id: string, body: WorkflowGraph) => api.put<WorkflowGraph>(`/workflows/${id}/graph`, body),
}

// Backward-compat alias — prefer workflowsAPI in new code
export const groupsAPI = workflowsAPI

export const webhookTriggersAPI = {
  list:   ()                         => api.get('/webhook-triggers'),
  create: (body: unknown)            => api.post('/webhook-triggers', body),
  get:    (id: string)               => api.get(`/webhook-triggers/${id}`),
  update: (id: string, body: unknown) => api.put(`/webhook-triggers/${id}`, body),
  delete: (id: string)               => api.delete(`/webhook-triggers/${id}`),
}

export const gatewayAPI = {
  listChannels: () => api.get('/gateway/channels'),
  createChannel: (body: unknown) => api.post('/gateway/channels', body),
  getChannel: (id: string) => api.get(`/gateway/channels/${id}`),
  updateChannel: (id: string, body: unknown) => api.put(`/gateway/channels/${id}`, body),
  deleteChannel: (id: string) => api.delete(`/gateway/channels/${id}`),
  adapterStatus: (id: string) => api.get(`/gateway/channels/${id}/adapter/status`),
  startLogin: (id: string) => api.post(`/gateway/channels/${id}/adapter/login/start`),
  getQR: (id: string) => api.get(`/gateway/channels/${id}/adapter/login/qr`),
  requestPairingCode: (id: string, phone: string) => api.post(`/gateway/channels/${id}/adapter/login/pairing-code`, { phone }),
  logout: (id: string) => api.post(`/gateway/channels/${id}/adapter/logout`),
  listSessions: (params?: string) => api.get(`/gateway/sessions${params ? '?' + params : ''}`),
  deleteSession: (id: string) => api.delete(`/gateway/sessions/${id}`),
  listEvents: (params?: string) => api.get(`/gateway/events${params ? '?' + params : ''}`),
  listPairings: (params?: string) => api.get(`/gateway/pairings${params ? '?' + params : ''}`),
  approvePairing: (id: string) => api.post(`/gateway/pairings/${id}/approve`),
  rejectPairing: (id: string) => api.post(`/gateway/pairings/${id}/reject`),
  listOutbox: (params?: string) => api.get(`/gateway/outbox${params ? '?' + params : ''}`),
  listReminders: (params?: string) => api.get(`/gateway/reminders${params ? '?' + params : ''}`),
  listEscalations: (params?: string) => api.get(`/gateway/escalations${params ? '?' + params : ''}`),
  approveEscalation: (id: string) => api.post(`/gateway/escalations/${id}/approve`),
  rejectEscalation: (id: string) => api.post(`/gateway/escalations/${id}/reject`),
  listContacts: (params?: string) => api.get(`/gateway/contacts${params ? '?' + params : ''}`),
  createContact: (body: unknown) => api.post('/gateway/contacts', body),
  updateContact: (id: string, body: unknown) => api.put(`/gateway/contacts/${id}`, body),
  deleteContact: (id: string) => api.delete(`/gateway/contacts/${id}`),
  listScheduledMessages: (params?: string) => api.get(`/gateway/scheduled-messages${params ? '?' + params : ''}`),
  createScheduledMessage: (body: unknown) => api.post('/gateway/scheduled-messages', body),
  deleteScheduledMessage: (id: string) => api.delete(`/gateway/scheduled-messages/${id}`),
}

export const skillsAPI = {
  list: () => api.get('/skills'),
  create: (body: unknown) => api.post('/skills', body),
  get: (id: string) => api.get(`/skills/${id}`),
  update: (id: string, body: unknown) => api.put(`/skills/${id}`, body),
  delete: (id: string) => api.delete(`/skills/${id}`),
  listForAgent: (agentId: string) => api.get(`/agents/${agentId}/skills`),
  setForAgent: (agentId: string, body: unknown) => api.put(`/agents/${agentId}/skills`, body),
}

export const nexusAIAPI = {
  chat: (
    messages: { role: 'user' | 'assistant'; content: string }[],
    opts?: { provider?: string; model?: string },
  ): Promise<Response> => {
    const token = typeof window !== 'undefined' ? localStorage.getItem('access_token') : null
    return fetch(`${API_URL}/api/v1/nexus-ai/chat`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      credentials: 'include',
      body: JSON.stringify({ messages, ...opts }),
    })
  },
}

export const apiTokensAPI = {
  list: () => api.get<{ data: APIToken[] }>('/api-tokens'),
  create: (body: { name: string; expires_at?: string | null; scopes?: string[] }) =>
    api.post<CreatedAPIToken>('/api-tokens', body),
  revoke: (id: string) => api.delete<void>(`/api-tokens/${id}`),
}

export const invokeAPI = {
  agent: (agentId: string, body: { input: string; conversation_id?: string; stream?: boolean }) =>
    api.post<InvokeRunResponse>(`/invoke/agents/${agentId}`, body),
  workflow: (workflowId: string, body: { input: string; stream?: boolean }) =>
    api.post<InvokeRunResponse>(`/invoke/workflows/${workflowId}`, body),
  /** @deprecated use invokeAPI.workflow instead */
  group: (workflowId: string, body: { input: string; stream?: boolean }) =>
    api.post<InvokeRunResponse>(`/invoke/workflows/${workflowId}`, body),
}

export const adminAPI = {
  users: () => api.get('/admin/users'),
  updateUser: (id: string, body: unknown) => api.patch(`/admin/users/${id}`, body),
  workspaces: () => api.get('/admin/workspaces'),
  updateWorkspace: (id: string, body: unknown) => api.patch(`/admin/workspaces/${id}`, body),
  auditLogs: (params?: string) => api.get(`/admin/audit-logs${params ? '?' + params : ''}`),
  serviceLogStream: () => api.sse('/admin/service-logs/stream'),
  usage: () => api.get('/admin/usage'),
  policies: () => api.get('/admin/policies'),
  setPolicies: (body: unknown) => api.put('/admin/policies', body),
}

export const configAPI = {
  get: (): Promise<{ demo_mode: boolean }> =>
    fetch(`${API_URL}/api/v1/config`).then((r) => r.json()),
}

export const observabilityAPI = {
  latency: (days = 7) => api.get(`/observability/latency?days=${days}`),
}
