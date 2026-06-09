'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronUp, ExternalLink, Globe, Lock, Plus, Server, Trash2, Wrench, X } from 'lucide-react'
import Link from 'next/link'
import { toolsAPI } from '@/lib/api'
import { riskColor } from '@/lib/utils'
import type { Tool } from '@/types'

// ─── types ───────────────────────────────────────────────────────────────────

interface KV { key: string; value: string }

interface HTTPForm {
  name: string
  description: string
  url: string
  method: string
  headers: KV[]
  queryParams: KV[]
  bodyMode: 'template' | 'free'
  bodyTemplate: string
  riskLevel: string
  requiresApproval: boolean
  timeoutMs: number
}

const METHOD_OPTIONS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'] as const
const RISK_OPTIONS   = ['low', 'medium', 'high', 'critical'] as const

const emptyForm = (): HTTPForm => ({
  name: '', description: '', url: '', method: 'POST',
  headers: [], queryParams: [],
  bodyMode: 'template', bodyTemplate: '',
  riskLevel: 'low', requiresApproval: false, timeoutMs: 30000,
})

// ─── helpers ─────────────────────────────────────────────────────────────────

const VAR_RE = /\{\{(\w+)\}\}/g

function extractVars(...sources: string[]): string[] {
  const seen = new Set<string>()
  for (const s of sources) {
    for (const m of s.matchAll(VAR_RE)) seen.add(m[1])
  }
  return [...seen]
}

function kvToObj(kvs: KV[]): Record<string, string> {
  return Object.fromEntries(kvs.filter((k) => k.key.trim()).map((k) => [k.key.trim(), k.value]))
}

function buildInputSchema(form: HTTPForm) {
  if (form.bodyMode === 'template') {
    const vars = extractVars(form.url, form.bodyTemplate,
      ...form.headers.map((h) => h.value),
      ...form.queryParams.map((q) => q.value))
    if (vars.length === 0) return { type: 'object', properties: {} }
    return {
      type: 'object',
      properties: Object.fromEntries(vars.map((v) => [v, { type: 'string' }])),
      required: vars,
    }
  }
  return {
    type: 'object',
    properties: {
      body: { type: 'object', description: 'Request body — provided in full by the LLM' },
    },
  }
}

// ─── small ui atoms ──────────────────────────────────────────────────────────

function KVEditor({ label, rows, onChange }: { label: string; rows: KV[]; onChange: (rows: KV[]) => void }) {
  const add = () => onChange([...rows, { key: '', value: '' }])
  const remove = (i: number) => onChange(rows.filter((_, j) => j !== i))
  const set = (i: number, field: keyof KV, val: string) =>
    onChange(rows.map((r, j) => (j === i ? { ...r, [field]: val } : r)))
  return (
    <div>
      <div className="flex items-center justify-between mb-1.5">
        <p className="text-[11px] font-medium text-gray-600">{label}</p>
        <button type="button" onClick={add} className="text-[11px] text-purple-600 hover:underline flex items-center gap-0.5">
          <Plus size={10} /> Add
        </button>
      </div>
      {rows.length === 0 && (
        <p className="text-[11px] text-gray-400 italic">None — click Add to set one.</p>
      )}
      {rows.map((r, i) => (
        <div key={i} className="flex items-center gap-1.5 mb-1.5">
          <input
            value={r.key} onChange={(e) => set(i, 'key', e.target.value)}
            placeholder="Key" className="flex-1 text-[12px] px-2 py-1.5 border border-gray-200 rounded-md bg-white font-mono"
          />
          <input
            value={r.value} onChange={(e) => set(i, 'value', e.target.value)}
            placeholder="Value (supports {{var}})" className="flex-[2] text-[12px] px-2 py-1.5 border border-gray-200 rounded-md bg-white font-mono"
          />
          <button type="button" onClick={() => remove(i)} className="text-gray-300 hover:text-red-400">
            <X size={13} />
          </button>
        </div>
      ))}
    </div>
  )
}

function Toggle({ on, onToggle }: { on: boolean; onToggle: () => void }) {
  return (
    <button onClick={onToggle} aria-label="toggle"
      className={`rounded-full relative flex-shrink-0 transition-colors ${on ? 'bg-purple-600' : 'bg-gray-200'}`}
      style={{ width: 32, height: 18 }}>
      <span className={`absolute top-0.5 w-3.5 h-3.5 bg-white rounded-full transition-all ${on ? 'left-[14px]' : 'left-0.5'}`} />
    </button>
  )
}

function typeInfo(type: string) {
  if (type === 'native') return { label: 'Built-in', icon: <Wrench size={11} />, cls: 'bg-blue-50 text-blue-700 border-blue-100' }
  if (type === 'mcp')    return { label: 'MCP', icon: <Server size={11} />, cls: 'bg-purple-50 text-purple-700 border-purple-100' }
  return                        { label: 'HTTP', icon: <Globe size={11} />, cls: 'bg-green-50 text-green-700 border-green-100' }
}

// ─── tool row ────────────────────────────────────────────────────────────────

function ToolRow({ tool, onToggle, onDelete }: { tool: Tool; onToggle: () => void; onDelete: () => void }) {
  const [open, setOpen] = useState(false)
  const ti = typeInfo(tool.type)
  const isSystem = tool.type === 'native' || tool.type === 'mcp'
  const cfg = tool.config ? (() => { try { return JSON.parse(tool.config as unknown as string) } catch { return null } })() : null

  return (
    <>
      <div className="flex items-center justify-between px-4 py-3 border-b last:border-b-0 border-gray-50 hover:bg-gray-50/50">
        <div className="flex items-center gap-3 min-w-0">
          <div className="w-7 h-7 rounded-lg bg-gray-100 flex items-center justify-center flex-shrink-0 text-gray-500">
            {ti.icon}
          </div>
          <div className="min-w-0">
            <p className="text-[13px] font-medium text-gray-900 font-mono truncate">{tool.name}</p>
            <p className="text-[11px] text-gray-400 mt-0.5 truncate">{tool.description || '—'}</p>
          </div>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0 ml-4">
          <span className={`inline-flex items-center gap-1 text-[10px] px-2 py-0.5 rounded-full border ${ti.cls}`}>
            {ti.icon}{ti.label}
          </span>
          <span className={`text-[10px] px-2 py-0.5 rounded-full border ${riskColor(tool.risk_level)}`}>
            {tool.risk_level}
          </span>
          {tool.requires_approval && (
            <span className="text-[10px] px-2 py-0.5 rounded-full bg-amber-50 text-amber-700 border border-amber-100">approval</span>
          )}
          {tool.type === 'http' && cfg?.url && (
            <button onClick={() => setOpen((v) => !v)} className="p-1 text-gray-400 hover:text-gray-600" title="Show config">
              {open ? <ChevronUp size={13} /> : <ChevronDown size={13} />}
            </button>
          )}
          {isSystem ? (
            <span title="Managed by system" className="p-1 text-gray-300"><Lock size={13} /></span>
          ) : (
            <>
              <Toggle on={tool.enabled} onToggle={onToggle} />
              <button onClick={onDelete} className="p-1 text-gray-300 hover:text-red-500" aria-label={`Delete ${tool.name}`}>
                <Trash2 size={13} />
              </button>
            </>
          )}
        </div>
      </div>
      {open && cfg && (
        <div className="px-4 py-3 bg-gray-50 border-b border-gray-100 text-[11px] text-gray-600 font-mono space-y-1">
          <p><span className="text-gray-400">URL</span>  {cfg.method} {cfg.url}</p>
          {cfg.body_mode && <p><span className="text-gray-400">body</span>  {cfg.body_mode === 'template' ? 'template' : 'free-form (LLM constructs)'}</p>}
          {cfg.body_template && (
            <pre className="mt-1 bg-white border border-gray-100 rounded p-2 overflow-x-auto whitespace-pre-wrap text-[10px]">{cfg.body_template}</pre>
          )}
        </div>
      )}
    </>
  )
}

// ─── http tool form ───────────────────────────────────────────────────────────

function HTTPToolForm({ onClose }: { onClose: () => void }) {
  const [form, setForm] = useState<HTTPForm>(emptyForm)
  const [error, setError] = useState('')
  const queryClient = useQueryClient()

  const detectedVars = extractVars(
    form.url, form.bodyTemplate,
    ...form.headers.map((h) => h.value),
    ...form.queryParams.map((q) => q.value),
  )

  const create = useMutation({
    mutationFn: () => {
      if (!form.name.trim()) throw new Error('Tool name is required')
      if (!form.url.trim())  throw new Error('URL is required')
      const inputSchema = buildInputSchema(form)
      const config = {
        url: form.url.trim(),
        method: form.method,
        headers: kvToObj(form.headers),
        query_params: kvToObj(form.queryParams),
        body_mode: form.bodyMode,
        body_template: form.bodyTemplate,
      }
      return toolsAPI.create({
        name: form.name.trim(),
        description: form.description.trim() || `${form.method} ${form.url.trim()}`,
        type: 'http',
        risk_level: form.riskLevel,
        requires_approval: form.requiresApproval,
        timeout_ms: form.timeoutMs,
        enabled: true,
        input_schema: inputSchema,
        output_schema: { type: 'object' },
        config,
      })
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['tools'] }); onClose() },
    onError: (err: Error) => setError(err.message),
  })

  const set = <K extends keyof HTTPForm>(k: K, v: HTTPForm[K]) => setForm((f) => ({ ...f, [k]: v }))

  return (
    <div className="border border-gray-200 rounded-xl p-5 mb-6 bg-white shadow-sm">
      <div className="flex items-center justify-between mb-4">
        <div>
          <p className="text-sm font-medium text-gray-900">New HTTP tool</p>
          <p className="text-[11px] text-gray-400 mt-0.5">
            Call any REST endpoint. Headers, query params, and the body can contain <code className="font-mono bg-gray-100 px-1 rounded">{`{{variable}}`}</code> tokens — the LLM fills them in at runtime.
          </p>
        </div>
        <button onClick={onClose}><X size={15} className="text-gray-400" /></button>
      </div>

      {error && <p className="text-[12px] text-red-600 bg-red-50 border border-red-100 rounded px-3 py-2 mb-3">{error}</p>}

      {/* Name + description */}
      <div className="grid grid-cols-2 gap-3 mb-4">
        <div>
          <label className="block text-[11px] font-medium text-gray-600 mb-1">Tool name *</label>
          <input value={form.name} onChange={(e) => set('name', e.target.value)}
            placeholder="e.g. send_slack_message" className="w-full text-sm px-3 py-2 border border-gray-200 rounded-lg font-mono" />
        </div>
        <div>
          <label className="block text-[11px] font-medium text-gray-600 mb-1">Description</label>
          <input value={form.description} onChange={(e) => set('description', e.target.value)}
            placeholder="What this tool does" className="w-full text-sm px-3 py-2 border border-gray-200 rounded-lg" />
        </div>
      </div>

      {/* URL + method */}
      <div className="mb-4">
        <label className="block text-[11px] font-medium text-gray-600 mb-1">Endpoint *</label>
        <div className="flex gap-2">
          <select value={form.method} onChange={(e) => set('method', e.target.value)}
            className="text-sm px-3 py-2 border border-gray-200 rounded-lg bg-white w-28 flex-shrink-0">
            {METHOD_OPTIONS.map((m) => <option key={m}>{m}</option>)}
          </select>
          <input value={form.url} onChange={(e) => set('url', e.target.value)}
            placeholder="https://api.example.com/endpoint  (supports {{var}})"
            className="flex-1 text-sm px-3 py-2 border border-gray-200 rounded-lg font-mono min-w-0" />
        </div>
      </div>

      {/* Headers */}
      <div className="mb-4">
        <KVEditor label="Headers" rows={form.headers} onChange={(v) => set('headers', v)} />
        <p className="text-[10px] text-gray-400 mt-1">
          Static headers (e.g. <code className="font-mono">Authorization: Bearer my-secret-key</code>) or dynamic ones with <code className="font-mono bg-gray-100 px-0.5 rounded">{`{{token}}`}</code>.
        </p>
      </div>

      {/* Query params */}
      <div className="mb-4">
        <KVEditor label="Query parameters" rows={form.queryParams} onChange={(v) => set('queryParams', v)} />
      </div>

      {/* Body mode */}
      {form.method !== 'GET' && form.method !== 'DELETE' && form.method !== 'HEAD' && (
        <div className="mb-4">
          <label className="block text-[11px] font-medium text-gray-600 mb-2">Request body</label>
          <div className="flex gap-2 mb-3">
            {(['template', 'free'] as const).map((mode) => (
              <button key={mode} type="button" onClick={() => set('bodyMode', mode)}
                className={`px-3 py-1.5 text-[12px] rounded-lg border transition-colors ${
                  form.bodyMode === mode
                    ? 'bg-purple-600 text-white border-purple-600'
                    : 'bg-white text-gray-600 border-gray-200 hover:border-gray-300'
                }`}>
                {mode === 'template' ? 'Template' : 'Free-form'}
              </button>
            ))}
          </div>

          {form.bodyMode === 'template' ? (
            <div>
              <p className="text-[11px] text-gray-500 mb-2">
                Write the JSON body you want to send. Use <code className="font-mono bg-gray-100 px-1 rounded">{`{{variable}}`}</code> for parts the LLM should fill in. The agent only needs to supply the variable values, not the full body.
              </p>
              <textarea
                value={form.bodyTemplate}
                onChange={(e) => set('bodyTemplate', e.target.value)}
                rows={5}
                placeholder={`{\n  "channel": "{{channel}}",\n  "text": "{{message}}"\n}`}
                className="w-full text-[12px] font-mono px-3 py-2 border border-gray-200 rounded-lg resize-y bg-white"
              />
              {detectedVars.length > 0 && (
                <div className="mt-2 flex flex-wrap gap-1.5">
                  <span className="text-[10px] text-gray-400">LLM will provide:</span>
                  {detectedVars.map((v) => (
                    <span key={v} className="text-[10px] px-2 py-0.5 bg-purple-50 text-purple-700 border border-purple-100 rounded-full font-mono">
                      {v}
                    </span>
                  ))}
                </div>
              )}
            </div>
          ) : (
            <div className="bg-gray-50 border border-gray-200 rounded-lg px-4 py-3">
              <p className="text-[12px] text-gray-600">
                The LLM constructs the entire body as a JSON object and passes it as <code className="font-mono bg-gray-100 px-1 rounded">body</code>. Use this when the payload is highly variable or hard to template.
              </p>
            </div>
          )}
        </div>
      )}

      {/* Risk + approval + timeout */}
      <div className="flex items-end gap-3 mb-4">
        <div>
          <label className="block text-[11px] font-medium text-gray-600 mb-1">Risk level</label>
          <select value={form.riskLevel} onChange={(e) => set('riskLevel', e.target.value)}
            className="text-sm px-3 py-2 border border-gray-200 rounded-lg bg-white">
            {RISK_OPTIONS.map((r) => <option key={r}>{r}</option>)}
          </select>
        </div>
        <div>
          <label className="block text-[11px] font-medium text-gray-600 mb-1">Timeout (ms)</label>
          <input type="number" value={form.timeoutMs} onChange={(e) => set('timeoutMs', Number(e.target.value))}
            className="text-sm px-3 py-2 border border-gray-200 rounded-lg w-28" />
        </div>
        <label className="flex items-center gap-2 text-[12px] text-gray-600 cursor-pointer pb-2">
          <input type="checkbox" checked={form.requiresApproval}
            onChange={(e) => set('requiresApproval', e.target.checked)} />
          Require approval before running
        </label>
      </div>

      {/* Actions */}
      <div className="flex gap-2">
        <button
          onClick={() => create.mutate()} disabled={create.isPending}
          className="px-4 py-2 bg-purple-600 text-white text-[12px] rounded-lg disabled:opacity-50 font-medium">
          {create.isPending ? 'Adding…' : 'Add tool'}
        </button>
        <button onClick={onClose} className="px-3 py-2 text-[12px] text-gray-500 hover:text-gray-800">Cancel</button>
      </div>
    </div>
  )
}

// ─── section ─────────────────────────────────────────────────────────────────

function Section({
  title, icon, blurb, emptyBlurb, tools, action, onToggle, onDelete,
}: {
  title: string; icon: React.ReactNode; blurb: string; emptyBlurb?: string
  tools: Tool[]; action?: React.ReactNode
  onToggle: (t: Tool) => void; onDelete: (t: Tool) => void
}) {
  return (
    <div className="mb-7">
      <div className="flex items-center justify-between mb-1">
        <div className="flex items-center gap-2">
          <span className="text-gray-400">{icon}</span>
          <p className="text-[13px] font-medium text-gray-800">{title}</p>
          <span className="text-[11px] text-gray-400 bg-gray-100 px-1.5 py-0.5 rounded-full">{tools.length}</span>
        </div>
        {action}
      </div>
      <p className="text-[11px] text-gray-400 ml-6 mb-2">{blurb}</p>
      {tools.length > 0 ? (
        <div className="bg-white border border-gray-100 rounded-xl overflow-hidden">
          {tools.map((t) => (
            <ToolRow key={t.id} tool={t} onToggle={() => onToggle(t)} onDelete={() => onDelete(t)} />
          ))}
        </div>
      ) : (
        <div className="border border-dashed border-gray-200 rounded-xl py-6 text-center">
          <p className="text-[12px] text-gray-400">{emptyBlurb ?? `No ${title.toLowerCase()} yet.`}</p>
        </div>
      )}
    </div>
  )
}

// ─── page ─────────────────────────────────────────────────────────────────────

export default function ToolsPage() {
  const queryClient = useQueryClient()
  const [addingHTTP, setAddingHTTP] = useState(false)
  const [actionError, setActionError] = useState('')

  const { data, isLoading, error } = useQuery({
    queryKey: ['tools'],
    queryFn: () => toolsAPI.list() as Promise<{ data: Tool[] }>,
  })
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['tools'] })
  const toggle = useMutation({
    mutationFn: (t: Tool) => toolsAPI.update(t.id, { enabled: !t.enabled }),
    onSuccess: refresh, onError: (e: Error) => setActionError(e.message),
  })
  const remove = useMutation({
    mutationFn: (id: string) => toolsAPI.delete(id),
    onSuccess: refresh, onError: (e: Error) => setActionError(e.message),
  })

  const tools = data?.data ?? []
  const native = tools.filter((t) => t.type === 'native')
  const mcp    = tools.filter((t) => t.type === 'mcp')
  const http   = tools.filter((t) => t.type === 'http')

  return (
    <div className="p-6 max-w-3xl">
      <div className="mb-6">
        <h1 className="text-base font-medium text-gray-900">Tool registry</h1>
        <p className="text-[12px] text-gray-400 mt-0.5">
          Tools let agents take actions beyond text — read files, call APIs, search the web.
        </p>
        <div className="flex items-center gap-3 text-[11px] mt-3">
          <span className="text-gray-400">Risk levels:</span>
          {([
            { level: 'low',      cls: 'bg-blue-50 text-blue-700 border-blue-100' },
            { level: 'medium',   cls: 'bg-amber-50 text-amber-700 border-amber-100' },
            { level: 'high',     cls: 'bg-orange-50 text-orange-700 border-orange-100' },
            { level: 'critical', cls: 'bg-red-50 text-red-700 border-red-100' },
          ] as const).map(({ level, cls }) => (
            <span key={level} className={`px-2 py-0.5 rounded-full border ${cls}`}>{level}</span>
          ))}
          <span className="text-gray-400 ml-1">
            · tools marked <span className="text-amber-700 font-medium">approval</span> pause the agent until a human approves
          </span>
        </div>
      </div>

      {(error || actionError) && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">
          {actionError || (error as Error).message}
        </div>
      )}

      {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading…</div>}

      {!isLoading && (
        <>
          {addingHTTP && <HTTPToolForm onClose={() => setAddingHTTP(false)} />}

          <Section
            title="Built-in tools"
            icon={<Wrench size={14} />}
            blurb="Platform tools built into Agent Nexus and always available. Assign them to agents from the agent's Tools tab."
            emptyBlurb="No built-in tools registered."
            tools={native}
            onToggle={(t) => toggle.mutate(t)}
            onDelete={(t) => { if (confirm(`Delete ${t.name}?`)) remove.mutate(t.id) }}
          />

          <Section
            title="MCP tools"
            icon={<Server size={14} />}
            blurb="Discovered from connected MCP servers. Manage which servers are connected to control which tools appear here."
            emptyBlurb="No MCP tools discovered yet. Connect an MCP server to populate this list."
            tools={mcp}
            action={
              <Link href="/mcp-servers" className="inline-flex items-center gap-1 text-[11px] text-purple-600 hover:underline">
                Manage MCP servers <ExternalLink size={10} />
              </Link>
            }
            onToggle={(t) => toggle.mutate(t)}
            onDelete={(t) => { if (confirm(`Delete ${t.name}?`)) remove.mutate(t.id) }}
          />

          <Section
            title="HTTP tools"
            icon={<Globe size={14} />}
            blurb="Call any REST endpoint. Use {{variable}} tokens in headers, query params, or the body — the agent fills them in at runtime."
            emptyBlurb="No HTTP tools yet. Add one to let agents call external REST APIs."
            tools={http}
            action={
              !addingHTTP ? (
                <button
                  onClick={() => { setAddingHTTP(true); setActionError('') }}
                  className="inline-flex items-center gap-1 text-[11px] text-purple-600 hover:underline">
                  <Plus size={11} /> Add HTTP tool
                </button>
              ) : undefined
            }
            onToggle={(t) => toggle.mutate(t)}
            onDelete={(t) => { if (confirm(`Delete ${t.name}?`)) remove.mutate(t.id) }}
          />
        </>
      )}
    </div>
  )
}
