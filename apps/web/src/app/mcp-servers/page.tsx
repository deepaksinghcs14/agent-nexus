'use client'

import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Globe, KeyRound, Package, Plug, Plus, RefreshCw, Terminal, Trash2, Wrench, X } from 'lucide-react'
import { mcpAPI } from '@/lib/api'
import { MCP_PRESETS, type MCPPreset } from '@/lib/mcp-presets'
import { relativeTime, riskColor, statusColor } from '@/lib/utils'
import { useDemoMode } from '@/context/demo-mode'
import type { MCPServer, MCPTool } from '@/types'

// ─── transport picker ─────────────────────────────────────────────────────────

const TRANSPORTS = [
  {
    id: 'http',
    label: 'HTTP + SSE',
    Icon: Globe,
    description: 'Connect to a running MCP server over HTTP. Provide a URL and Agent Nexus will connect via SSE.',
  },
  {
    id: 'stdio',
    label: 'stdio',
    Icon: Terminal,
    description: 'Spawn a local MCP process. Agent Nexus starts the process and communicates over stdin/stdout.',
  },
]

// ─── add server panel ─────────────────────────────────────────────────────────

type CreateBody = {
  name: string
  url: string
  transport: string
  config?: { token?: string; env?: Record<string, string> }
}

function AddServerPanel({
  onClose,
  onCreate,
  isPending,
}: {
  onClose: () => void
  onCreate: (body: CreateBody) => void
  isPending: boolean
}) {
  const [form, setForm] = useState({ name: '', url: '', transport: 'http', token: '' })
  const [formError, setFormError] = useState('')
  // null = custom setup; otherwise a preset from the catalog drives the form.
  const [preset, setPreset] = useState<MCPPreset | null>(null)
  const [envValues, setEnvValues] = useState<Record<string, string>>({})

  const pickPreset = (p: MCPPreset | null) => {
    setPreset(p)
    setEnvValues({})
    setFormError('')
    if (p) setForm((f) => ({ ...f, name: p.label, url: p.command, transport: 'stdio' }))
    else setForm({ name: '', url: '', transport: 'http', token: '' })
  }

  const handleSubmit = () => {
    if (!form.name.trim()) { setFormError('Server name is required'); return }
    if (!form.url.trim())  { setFormError('URL / command is required'); return }
    if (preset) {
      const missing = preset.env.filter((f) => f.required && !envValues[f.key]?.trim())
      if (missing.length > 0) {
        setFormError(`${missing.map((f) => f.label).join(', ')} ${missing.length === 1 ? 'is' : 'are'} required`)
        return
      }
    }
    setFormError('')
    const body: CreateBody = {
      name: form.name.trim(),
      url: form.url.trim(),
      transport: form.transport,
    }
    if (preset) {
      const env: Record<string, string> = {}
      for (const f of preset.env) {
        const v = envValues[f.key]?.trim()
        if (v) env[f.key] = v
      }
      if (Object.keys(env).length > 0) body.config = { env }
    } else if (form.transport === 'http' && form.token.trim()) {
      body.config = { token: form.token.trim() }
    }
    onCreate(body)
  }

  const selectedTransport = TRANSPORTS.find((t) => t.id === form.transport)!

  return (
    <div className="border border-border-strong rounded-xl mb-6 overflow-hidden bg-surface">
      <div className="flex items-center justify-between px-4 py-3 border-b border-border bg-muted">
        <p className="text-sm font-medium text-foreground">Add MCP server</p>
        <button onClick={onClose}><X size={14} className="text-faint" /></button>
      </div>

      <div className="p-4">
        {formError && (
          <p className="text-[12px] text-crit bg-crit/10 border border-crit/30 rounded px-3 py-2 mb-3">{formError}</p>
        )}

        {/* Preset catalog: popular token-auth servers, or fully custom */}
        <p className="text-[11px] font-medium text-faint uppercase tracking-wide mb-3">Server</p>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2 mb-4">
          {MCP_PRESETS.map((p) => {
            const active = preset?.id === p.id
            return (
              <button
                key={p.id}
                onClick={() => pickPreset(active ? null : p)}
                className={`flex items-start gap-2.5 p-3 rounded-xl border text-left transition-all ${
                  active ? 'border-accent/50 bg-accent/10' : 'border-border-strong hover:border-border-strong bg-surface'
                }`}
              >
                <Package size={14} className={`mt-0.5 flex-shrink-0 ${active ? 'text-accent dark:text-accent-bright' : 'text-faint'}`} />
                <div>
                  <p className={`text-[12px] font-medium ${active ? 'text-accent dark:text-accent-bright' : 'text-foreground'}`}>{p.label}</p>
                  <p className="text-[10px] text-faint leading-tight mt-0.5">{p.description}</p>
                </div>
              </button>
            )
          })}
          <button
            onClick={() => pickPreset(null)}
            className={`flex items-start gap-2.5 p-3 rounded-xl border text-left transition-all ${
              preset === null ? 'border-accent/50 bg-accent/10' : 'border-border-strong hover:border-border-strong bg-surface'
            }`}
          >
            <Wrench size={14} className={`mt-0.5 flex-shrink-0 ${preset === null ? 'text-accent dark:text-accent-bright' : 'text-faint'}`} />
            <div>
              <p className={`text-[12px] font-medium ${preset === null ? 'text-accent dark:text-accent-bright' : 'text-foreground'}`}>Custom</p>
              <p className="text-[10px] text-faint leading-tight mt-0.5">Any HTTP or stdio MCP server — bring your own URL or command.</p>
            </div>
          </button>
        </div>

        {preset ? (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
              <div>
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Server name *</label>
                <input
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                  className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg"
                />
              </div>
              <div>
                <label className="block text-[11px] font-medium text-muted-foreground mb-1">Command</label>
                <input
                  value={form.url}
                  readOnly
                  className="w-full text-sm px-3 py-2 border border-border rounded-lg font-mono bg-muted text-muted-foreground"
                />
              </div>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
              {preset.env.map((f) => (
                <div key={f.key}>
                  <label className="block text-[11px] font-medium text-muted-foreground mb-1">
                    {f.label}{f.required ? ' *' : ''}{' '}
                    <code className="text-[10px] text-faint font-normal">{f.key}</code>
                  </label>
                  <input
                    type={f.secret ? 'password' : 'text'}
                    value={envValues[f.key] ?? ''}
                    onChange={(e) => setEnvValues((v) => ({ ...v, [f.key]: e.target.value }))}
                    placeholder={f.placeholder}
                    className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg font-mono"
                  />
                </div>
              ))}
            </div>
            <p className="text-[11px] text-faint mb-4">
              {preset.note ? `${preset.note} ` : ''}
              Credentials are stored encrypted and injected as environment variables into the server process.
              {preset.docsUrl && (
                <> Docs: <a href={preset.docsUrl} target="_blank" rel="noreferrer" className="text-accent dark:text-accent-bright hover:underline break-all">{preset.docsUrl}</a></>
              )}
            </p>
          </>
        ) : (
          <>
        {/* Transport picker */}
        <p className="text-[11px] font-medium text-faint uppercase tracking-wide mb-3">Transport</p>
        <div className="grid grid-cols-2 gap-2 mb-4">
          {TRANSPORTS.map(({ id, label, Icon, description }) => {
            const active = form.transport === id
            return (
              <button
                key={id}
                onClick={() => setForm((f) => ({ ...f, transport: id }))}
                className={`flex items-start gap-2.5 p-3 rounded-xl border text-left transition-all ${
                  active ? 'border-accent/50 bg-accent/10' : 'border-border-strong hover:border-border-strong bg-surface'
                }`}
              >
                <Icon size={14} className={`mt-0.5 flex-shrink-0 ${active ? 'text-accent dark:text-accent-bright' : 'text-faint'}`} />
                <div>
                  <p className={`text-[12px] font-medium ${active ? 'text-accent dark:text-accent-bright' : 'text-foreground'}`}>{label}</p>
                  <p className="text-[10px] text-faint leading-tight mt-0.5">{description}</p>
                </div>
              </button>
            )
          })}
        </div>

        {/* Fields */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-3">
          <div>
            <label className="block text-[11px] font-medium text-muted-foreground mb-1">Server name *</label>
            <input
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              placeholder="e.g. Filesystem MCP"
              className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg"
            />
          </div>
          <div>
            <label className="block text-[11px] font-medium text-muted-foreground mb-1">
              {form.transport === 'http' ? 'Server URL *' : 'Command *'}
            </label>
            <input
              value={form.url}
              onChange={(e) => setForm((f) => ({ ...f, url: e.target.value }))}
              placeholder={form.transport === 'http' ? 'https://mcp.example.com' : 'npx @modelcontextprotocol/server-filesystem /path'}
              className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg font-mono"
            />
          </div>
        </div>

        {form.transport === 'http' && (
          <div className="mb-3">
            <label className="block text-[11px] font-medium text-muted-foreground mb-1">
              Auth token <span className="text-faint font-normal">(optional — sent as <code className="bg-muted px-1 rounded">Authorization: Bearer …</code>)</span>
            </label>
            <input
              type="password"
              value={form.token}
              onChange={(e) => setForm((f) => ({ ...f, token: e.target.value }))}
              placeholder="sk-… or leave blank if no auth required"
              className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg font-mono"
            />
          </div>
        )}

        <p className="text-[11px] text-faint mb-4">
          {selectedTransport.description} After adding, click <strong>Sync</strong> to discover available tools.
        </p>
          </>
        )}

        <div className="flex gap-2">
          <button
            onClick={handleSubmit}
            disabled={isPending}
            className="px-4 py-1.5 bg-accent text-white text-[12px] rounded-lg font-medium disabled:opacity-50"
          >
            {isPending ? 'Adding…' : 'Add server'}
          </button>
          <button onClick={onClose} className="px-3 py-1.5 text-[12px] text-muted-foreground hover:text-foreground dark:hover:text-faint">
            Cancel
          </button>
        </div>
      </div>
    </div>
  )
}

// ─── risk row ────────────────────────────────────────────────────────────────

const RISK_LEVELS = ['low', 'medium', 'high', 'critical'] as const
type RiskLevel = typeof RISK_LEVELS[number]

function RiskRow({ tool, serverId, onRiskChange }: { tool: MCPTool; serverId: string; onRiskChange: () => void }) {
  const [saving, setSaving] = useState(false)

  const handleRisk = async (level: RiskLevel) => {
    setSaving(true)
    try {
      await mcpAPI.updateToolRisk(serverId, tool.id, level)
      onRiskChange()
    } finally {
      setSaving(false)
    }
  }

  return (
    <tr className="border-b last:border-b-0 border-border hover:bg-muted/50">
      <td className="px-4 py-2.5">
        <p className="text-[12px] font-medium text-foreground font-mono">{tool.name}</p>
      </td>
      <td className="px-4 py-2.5">
        <p className="text-[11px] text-muted-foreground truncate max-w-xs">{tool.description || '—'}</p>
      </td>
      <td className="px-4 py-2.5">
        <select
          value={tool.risk_level}
          disabled={saving}
          onChange={(e) => handleRisk(e.target.value as RiskLevel)}
          className={`text-[10px] px-1.5 py-0.5 rounded border bg-transparent cursor-pointer disabled:opacity-50 ${riskColor(tool.risk_level)}`}
        >
          {RISK_LEVELS.map((l) => (
            <option key={l} value={l}>{l}</option>
          ))}
        </select>
      </td>
    </tr>
  )
}

// ─── status dot ───────────────────────────────────────────────────────────────

function StatusDot({ status }: { status: string }) {
  const cls =
    status === 'connected'    ? 'bg-good' :
    status === 'error'        ? 'bg-crit' :
    status === 'disconnected' ? 'bg-faint' :
                                'bg-warn'
  return <span className={`w-2 h-2 rounded-full flex-shrink-0 ${cls}`} />
}

// ─── page ─────────────────────────────────────────────────────────────────────

export default function MCPServersPage() {
  const queryClient = useQueryClient()
  const demoMode = useDemoMode()
  const [selected, setSelected] = useState('')
  const [showAdd, setShowAdd] = useState(false)
  const [actionError, setActionError] = useState('')

  const { data, isLoading, error } = useQuery({
    queryKey: ['mcp-servers'],
    queryFn: () => mcpAPI.list() as Promise<{ data: MCPServer[] }>,
  })
  const servers = data?.data ?? []

  // Auto-select first server
  useEffect(() => {
    if (!selected && servers[0]) setSelected(servers[0].id)
  }, [selected, servers])

  const selectedServer = servers.find((s) => s.id === selected)

  const { data: toolsData, isLoading: toolsLoading, error: toolsError } = useQuery({
    queryKey: ['mcp-tools', selected],
    queryFn: () => mcpAPI.tools(selected) as Promise<{ data: MCPTool[] }>,
    enabled: !!selected,
  })

  const create = useMutation({
    mutationFn: (body: CreateBody) => mcpAPI.create(body),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['mcp-servers'] }); setShowAdd(false); setActionError('') },
    onError: (err: Error) => setActionError(err.message),
  })
  const sync = useMutation({
    mutationFn: (id: string) => mcpAPI.sync(id),
    onSuccess: (_, id) => {
      queryClient.invalidateQueries({ queryKey: ['mcp-servers'] })
      queryClient.invalidateQueries({ queryKey: ['mcp-tools', id] })
    },
    onError: (err: Error) => setActionError(err.message),
  })
  const remove = useMutation({
    mutationFn: (id: string) => mcpAPI.delete(id),
    onSuccess: () => { setSelected(''); queryClient.invalidateQueries({ queryKey: ['mcp-servers'] }) },
    onError: (err: Error) => setActionError(err.message),
  })
  const oauthConnect = useMutation({
    mutationFn: (id: string) => mcpAPI.oauthStart(id) as Promise<{ authorize_url: string }>,
    onSuccess: (res) => {
      setActionError('')
      // The user grants access in the provider's consent screen; the callback
      // lands on the API, which stores tokens and syncs tools automatically.
      window.open(res.authorize_url, '_blank', 'noopener')
      setTimeout(() => {
        queryClient.invalidateQueries({ queryKey: ['mcp-servers'] })
        queryClient.invalidateQueries({ queryKey: ['mcp-tools'] })
      }, 15000)
    },
    onError: (err: Error) => setActionError(err.message),
  })

  const tools = toolsData?.data ?? []

  return (
    <div className="p-4 sm:p-6 max-w-4xl">
      {/* Header */}
      <div className="flex flex-wrap items-center gap-3 justify-between mb-5">
        <div>
          <span className="eyebrow block mb-1">Integrations</span>
          <h1 className="text-[22px] font-bold tracking-tight text-foreground">MCP Servers</h1>
          <p className="text-[12px] text-faint mt-0.5">
            Connect Model Context Protocol servers to give agents access to external tools.
          </p>
        </div>
        {servers.length > 0 && !showAdd && (
          <button
            onClick={() => { setShowAdd(true); setActionError('') }}
            disabled={demoMode}
            title={demoMode ? 'Not available in demo mode' : undefined}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-accent text-white text-[12px] rounded-lg disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <Plus size={13} /> Add server
          </button>
        )}
      </div>

      {(error || actionError) && (
        <div className="text-sm text-crit bg-crit/10 border border-crit/30 rounded-lg p-3 mb-4">
          {actionError || (error as Error).message}
        </div>
      )}

      {showAdd && (
        <AddServerPanel
          onClose={() => setShowAdd(false)}
          onCreate={(body) => create.mutate(body)}
          isPending={create.isPending}
        />
      )}

      {isLoading && <div className="py-12 text-center text-sm text-faint">Loading…</div>}

      {/* Empty state */}
      {!isLoading && !error && servers.length === 0 && !showAdd && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-8 border border-dashed border-border-strong rounded-xl p-8">
          <div>
            <Plug size={22} className="text-accent dark:text-accent-bright mb-3" />
            <p className="text-sm font-medium text-foreground mb-1">What are MCP servers?</p>
            <p className="text-[12px] text-muted-foreground leading-relaxed mb-5">
              MCP (Model Context Protocol) is an open standard for connecting LLMs to external tools and data sources. Any MCP-compatible server exposes a list of tools that agents can discover and call — databases, APIs, file systems, and more.
            </p>
            <div className="space-y-3">
              {[
                { n: '1', title: 'Add a server',       desc: 'Connect via HTTP+SSE or spawn a local stdio process' },
                { n: '2', title: 'Sync tools',          desc: 'Agent Nexus queries the server and imports its tool list' },
                { n: '3', title: 'Assign to agents',    desc: 'MCP tools appear in the Tools tab of any agent' },
              ].map((s) => (
                <div key={s.n} className="flex items-start gap-2.5">
                  <span className="w-5 h-5 rounded-full bg-accent/15 text-accent dark:text-accent-bright text-[10px] font-bold flex items-center justify-center flex-shrink-0 mt-0.5">
                    {s.n}
                  </span>
                  <div>
                    <p className="text-[12px] font-medium text-foreground">{s.title}</p>
                    <p className="text-[11px] text-faint">{s.desc}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>
          <div className="flex flex-col items-center justify-center sm:border-l border-dashed border-border-strong sm:pl-8 pt-4 sm:pt-0 border-t sm:border-t-0">
            <Wrench size={28} className="text-faint mb-3" />
            <p className="text-sm font-medium text-muted-foreground mb-1">Connect your first server</p>
            <p className="text-[11px] text-faint mb-5 text-center">
              Use any MCP-compatible server — community servers, your own, or the official MCP examples.
            </p>
            <button
              onClick={() => { setShowAdd(true); setActionError('') }}
              disabled={demoMode}
              title={demoMode ? 'Not available in demo mode' : undefined}
              className="px-4 py-2 bg-accent text-white text-[12px] rounded-lg font-medium disabled:opacity-40 disabled:cursor-not-allowed"
            >
              {demoMode ? 'Not available in demo' : 'Add server'}
            </button>
          </div>
        </div>
      )}

      {/* Server cards */}
      {servers.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 mb-6">
          {servers.map((server) => {
            const isSyncing = sync.isPending && (sync.variables as string) === server.id
            const TransportIcon = server.transport === 'stdio' ? Terminal : Globe
            return (
              <div
                key={server.id}
                onClick={() => setSelected(server.id === selected ? '' : server.id)}
                className={`bg-surface border shadow-card rounded-xl p-3.5 cursor-pointer transition-all hover:-translate-y-0.5 ${
                  selected === server.id
                    ? 'border-accent/50 ring-1 ring-accent/20'
                    : 'border-border hover:border-border-strong'
                }`}
              >
                <div className="flex items-start justify-between mb-2">
                  <div className="flex items-center gap-1.5 min-w-0">
                    <StatusDot status={server.status} />
                    <p className="text-[13px] font-medium text-foreground truncate">{server.name}</p>
                  </div>
                  {server.status === 'error' && (
                    <AlertTriangle size={13} className="text-crit flex-shrink-0 ml-1" />
                  )}
                </div>

                <p className="text-[10px] font-mono text-faint truncate mb-2" title={server.url}>
                  {server.url}
                </p>

                <div className="flex items-center gap-1.5 mb-3">
                  <span className={`inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded border ${statusColor(server.status)} `}>
                    {server.status}
                  </span>
                  <span className="inline-flex items-center gap-1 text-[10px] text-faint">
                    <TransportIcon size={10} />
                    {server.transport}
                  </span>
                  {server.auth_type === 'oauth' && (
                    <span className="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded border bg-info/10 text-info border-info/30">
                      oauth
                    </span>
                  )}
                  {server.tools_synced_at && (
                    <span className="text-[10px] text-faint ml-auto">
                      synced {relativeTime(server.tools_synced_at)}
                    </span>
                  )}
                </div>

                <div className="flex flex-wrap gap-1.5" onClick={(e) => e.stopPropagation()}>
                  <button
                    onClick={() => sync.mutate(server.id)}
                    disabled={isSyncing}
                    className="inline-flex items-center gap-1 px-2 py-1 border border-border-strong text-[11px] text-muted-foreground rounded-md hover:bg-muted disabled:opacity-50"
                  >
                    <RefreshCw size={11} className={isSyncing ? 'animate-spin' : ''} />
                    {isSyncing ? 'Syncing…' : 'Sync tools'}
                  </button>
                  {server.transport !== 'stdio' && (
                    <button
                      onClick={() => oauthConnect.mutate(server.id)}
                      disabled={oauthConnect.isPending && (oauthConnect.variables as string) === server.id}
                      title="Authorize via OAuth (for servers like Atlassian's hosted MCP)"
                      className="inline-flex items-center gap-1 px-2 py-1 border border-info/30 text-[11px] text-info rounded-md hover:bg-info/10 disabled:opacity-50"
                    >
                      <KeyRound size={11} />
                      {server.auth_type === 'oauth' ? 'Re-connect' : 'Connect (OAuth)'}
                    </button>
                  )}
                  <button
                    onClick={() => { if (confirm('Delete this server and all its discovered tools?')) remove.mutate(server.id) }}
                    className="ml-auto p-1 text-faint hover:text-crit"
                  >
                    <Trash2 size={13} />
                  </button>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {/* Tools panel */}
      {selectedServer && (
        <div className="bg-surface border border-border rounded-xl overflow-hidden">
          <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3 border-b border-border">
            <div className="flex items-center gap-2">
              <Wrench size={13} className="text-faint" />
              <p className="text-[12px] font-medium text-foreground">
                Discovered tools — {selectedServer.name}
              </p>
              {!toolsLoading && (
                <span className="text-[11px] text-faint bg-muted px-1.5 py-0.5 rounded-full">{tools.length}</span>
              )}
            </div>
            {selectedServer.tools_synced_at ? (
              <p className="text-[11px] text-faint">
                Last synced {relativeTime(selectedServer.tools_synced_at)}
              </p>
            ) : (
              <p className="text-[11px] text-faint">Not yet synced</p>
            )}
          </div>

          {toolsError && (
            <div className="text-xs text-crit bg-crit/10 px-4 py-3">{(toolsError as Error).message}</div>
          )}
          {toolsLoading && <div className="py-8 text-center text-sm text-faint">Loading tools…</div>}

          {!toolsLoading && !toolsError && tools.length === 0 && (
            <div className="py-10 text-center">
              <Wrench size={20} className="text-faint mx-auto mb-2" />
              <p className="text-[12px] text-faint">No tools discovered yet.</p>
              <p className="text-[11px] text-faint mt-1">
                Click <strong>Sync tools</strong> on the server card to query the server.
              </p>
            </div>
          )}

          {tools.length > 0 && (
            <div className="overflow-x-auto">
            <table className="w-full min-w-[480px]">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left text-[10px] font-medium text-faint px-4 py-2">Tool name</th>
                  <th className="text-left text-[10px] font-medium text-faint px-4 py-2">Description</th>
                  <th className="text-left text-[10px] font-medium text-faint px-4 py-2 w-32">Risk</th>
                </tr>
              </thead>
              <tbody>
                {tools.map((tool) => (
                  <RiskRow
                    key={tool.id}
                    tool={tool}
                    serverId={selected}
                    onRiskChange={() => queryClient.invalidateQueries({ queryKey: ['mcp-tools', selected] })}
                  />
                ))}
              </tbody>
            </table>
            </div>
          )}
        </div>
      )}

      <div className="mt-6 p-4 rounded-lg bg-muted border border-border-strong">
        <p className="text-sm text-muted-foreground">
          Learn how to connect MCP servers and manage tool discovery in the{' '}
          <a href="/docs/mcp-servers" className="text-accent dark:text-accent-bright hover:underline">
            documentation
          </a>.
        </p>
      </div>
    </div>
  )
}
