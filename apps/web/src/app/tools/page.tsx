'use client'

import { useState } from 'react'
import dynamic from 'next/dynamic'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronUp, ExternalLink, Globe, Lock, Pencil, Play, Plus, Server, Terminal, Trash2, Wrench, X } from 'lucide-react'
import Link from 'next/link'
import { toolsAPI } from '@/lib/api'
import { riskColor } from '@/lib/utils'
import type { Tool } from '@/types'

// ─── CodeMirror (dynamic — client only, avoids SSR window references) ────────

const CodeEditor = dynamic(() => import('@/components/CodeEditor'), { ssr: false, loading: () => (
  <div className="h-48 bg-gray-900 rounded-lg flex items-center justify-center text-sm text-muted-foreground">Loading editor…</div>
)})

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

interface CodeForm {
  name: string
  description: string
  code: string
  inputSchema: string
  riskLevel: string
  requiresApproval: boolean
  timeoutMs: number
}

const METHOD_OPTIONS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'] as const
const RISK_OPTIONS   = ['low', 'medium', 'high', 'critical'] as const

const DEFAULT_CODE = `// Receives 'input' (object) — return any JSON-serializable value.
function main(input) {
  return { message: "Hello from " + (input.name ?? "agent") };
}

main(input)
`

const emptyHTTPForm = (): HTTPForm => ({
  name: '', description: '', url: '', method: 'POST',
  headers: [], queryParams: [],
  bodyMode: 'template', bodyTemplate: '',
  riskLevel: 'low', requiresApproval: false, timeoutMs: 30000,
})

const emptyCodeForm = (): CodeForm => ({
  name: '', description: '', code: DEFAULT_CODE,
  inputSchema: '{\n  "type": "object",\n  "properties": {\n    "name": { "type": "string" }\n  }\n}',
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
        <p className="text-[11px] font-medium text-muted-foreground">{label}</p>
        <button type="button" onClick={add} className="text-[11px] text-accent dark:text-accent-bright hover:underline flex items-center gap-0.5">
          <Plus size={10} /> Add
        </button>
      </div>
      {rows.length === 0 && (
        <p className="text-[11px] text-faint italic">None — click Add to set one.</p>
      )}
      {rows.map((r, i) => (
        <div key={i} className="flex items-center gap-1.5 mb-1.5">
          <input
            value={r.key} onChange={(e) => set(i, 'key', e.target.value)}
            placeholder="Key" className="flex-1 text-[12px] px-2 py-1.5 border border-border-strong rounded-md bg-surface font-mono"
          />
          <input
            value={r.value} onChange={(e) => set(i, 'value', e.target.value)}
            placeholder="Value (supports {{var}})" className="flex-[2] text-[12px] px-2 py-1.5 border border-border-strong rounded-md bg-surface font-mono"
          />
          <button type="button" onClick={() => remove(i)} className="text-faint hover:text-red-400">
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
      className={`rounded-full relative flex-shrink-0 transition-colors ${on ? 'bg-accent' : 'bg-gray-200'}`}
      style={{ width: 32, height: 18 }}>
      <span className={`absolute top-0.5 w-3.5 h-3.5 bg-surface rounded-full transition-all ${on ? 'left-[14px]' : 'left-0.5'}`} />
    </button>
  )
}

function typeInfo(type: string) {
  if (type === 'native') return { label: 'Built-in',    icon: <Wrench size={11} />,   cls: 'bg-blue-50 dark:bg-blue-500/10 text-blue-700 dark:text-blue-300 border-blue-100' }
  if (type === 'mcp')    return { label: 'MCP',         icon: <Server size={11} />,   cls: 'bg-accent/10 text-accent dark:text-accent-bright border-accent/25' }
  if (type === 'code')   return { label: 'Code',        icon: <Terminal size={11} />, cls: 'bg-orange-50 dark:bg-orange-500/10 text-orange-700 dark:text-orange-300 border-orange-100' }
  return                        { label: 'HTTP',        icon: <Globe size={11} />,    cls: 'bg-green-50 dark:bg-green-500/10 text-green-700 dark:text-green-300 border-green-100' }
}

// ─── tool row ────────────────────────────────────────────────────────────────

function ToolRow({ tool, onToggle, onDelete, onEdit }: {
  tool: Tool; onToggle: () => void; onDelete: () => void; onEdit?: () => void
}) {
  const [open, setOpen] = useState(false)
  const ti = typeInfo(tool.type)
  const isSystem = tool.type === 'native' || tool.type === 'mcp'
  const cfg = tool.config ? (() => { try { return JSON.parse(tool.config as unknown as string) } catch { return null } })() : null
  const canExpand = (tool.type === 'http' && cfg?.url) || tool.type === 'code'

  return (
    <>
      <div className="flex items-center justify-between px-4 py-3 border-b last:border-b-0 border-gray-50 hover:bg-gray-50/50">
        <div className="flex items-center gap-3 min-w-0">
          <div className="w-7 h-7 rounded-lg bg-muted flex items-center justify-center flex-shrink-0 text-muted-foreground">
            {ti.icon}
          </div>
          <div className="min-w-0">
            <p className="text-[13px] font-medium text-foreground font-mono truncate">{tool.name}</p>
            <p className="text-[11px] text-faint mt-0.5 truncate">{tool.description || '—'}</p>
          </div>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0 ml-2 overflow-x-auto">
          <span className={`inline-flex items-center gap-1 text-[10px] px-2 py-0.5 rounded-full border flex-shrink-0 ${ti.cls}`}>
            {ti.icon}{ti.label}
          </span>
          <span className={`text-[10px] px-2 py-0.5 rounded-full border ${riskColor(tool.risk_level)}`}>
            {tool.risk_level}
          </span>
          {tool.requires_approval && (
            <span className="text-[10px] px-2 py-0.5 rounded-full bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300 border border-amber-100">approval</span>
          )}
          {canExpand && (
            <button onClick={() => setOpen((v) => !v)} className="p-1 text-faint hover:text-gray-600 dark:hover:text-gray-300" title="Show code">
              {open ? <ChevronUp size={13} /> : <ChevronDown size={13} />}
            </button>
          )}
          {isSystem ? (
            <span title="Managed by system" className="p-1 text-faint"><Lock size={13} /></span>
          ) : (
            <>
              {onEdit && tool.type === 'code' && (
                <button onClick={onEdit} className="p-1 text-faint hover:text-blue-500" title="Edit code tool">
                  <Pencil size={13} />
                </button>
              )}
              <Toggle on={tool.enabled} onToggle={onToggle} />
              <button onClick={onDelete} className="p-1 text-faint hover:text-red-500" aria-label={`Delete ${tool.name}`}>
                <Trash2 size={13} />
              </button>
            </>
          )}
        </div>
      </div>
      {open && (
        <div className="border-b border-border">
          {tool.type === 'http' && cfg && (
            <div className="px-4 py-3 bg-muted text-[11px] text-muted-foreground font-mono space-y-1">
              <p><span className="text-faint">URL</span>  {cfg.method} {cfg.url}</p>
              {cfg.body_mode && <p><span className="text-faint">body</span>  {cfg.body_mode === 'template' ? 'template' : 'free-form (LLM constructs)'}</p>}
              {cfg.body_template && (
                <pre className="mt-1 bg-surface border border-border rounded p-2 overflow-x-auto whitespace-pre-wrap text-[10px]">{cfg.body_template}</pre>
              )}
            </div>
          )}
          {tool.type === 'code' && cfg?.code && (
            <pre className="px-4 py-3 bg-gray-900 text-green-300 text-[11px] font-mono overflow-x-auto whitespace-pre-wrap leading-relaxed">
              {cfg.code}
            </pre>
          )}
        </div>
      )}
    </>
  )
}

// ─── http tool form ───────────────────────────────────────────────────────────

function HTTPToolForm({ onClose }: { onClose: () => void }) {
  const [form, setForm] = useState<HTTPForm>(emptyHTTPForm)
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
    <div className="border border-border-strong rounded-xl p-5 mb-6 bg-surface shadow-sm">
      <div className="flex items-center justify-between mb-4">
        <div>
          <p className="text-sm font-medium text-foreground">New HTTP tool</p>
          <p className="text-[11px] text-faint mt-0.5">
            Call any REST endpoint. Headers, query params, and the body can contain <code className="font-mono bg-muted px-1 rounded">{`{{variable}}`}</code> tokens — the LLM fills them in at runtime.
          </p>
        </div>
        <button onClick={onClose}><X size={15} className="text-faint" /></button>
      </div>

      {error && <p className="text-[12px] text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-100 rounded px-3 py-2 mb-3">{error}</p>}

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-4">
        <div>
          <label className="block text-[11px] font-medium text-muted-foreground mb-1">Tool name *</label>
          <input value={form.name} onChange={(e) => set('name', e.target.value)}
            placeholder="e.g. send_slack_message" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg font-mono" />
        </div>
        <div>
          <label className="block text-[11px] font-medium text-muted-foreground mb-1">Description</label>
          <input value={form.description} onChange={(e) => set('description', e.target.value)}
            placeholder="What this tool does" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg" />
        </div>
      </div>

      <div className="mb-4">
        <label className="block text-[11px] font-medium text-muted-foreground mb-1">Endpoint *</label>
        <div className="flex gap-2">
          <select value={form.method} onChange={(e) => set('method', e.target.value)}
            className="text-sm px-3 py-2 border border-border-strong rounded-lg bg-surface w-28 flex-shrink-0">
            {METHOD_OPTIONS.map((m) => <option key={m}>{m}</option>)}
          </select>
          <input value={form.url} onChange={(e) => set('url', e.target.value)}
            placeholder="https://api.example.com/endpoint  (supports {{var}})"
            className="flex-1 text-sm px-3 py-2 border border-border-strong rounded-lg font-mono min-w-0" />
        </div>
      </div>

      <div className="mb-4">
        <KVEditor label="Headers" rows={form.headers} onChange={(v) => set('headers', v)} />
        <p className="text-[10px] text-faint mt-1">
          Static headers (e.g. <code className="font-mono">Authorization: Bearer my-secret-key</code>) or dynamic ones with <code className="font-mono bg-muted px-0.5 rounded">{`{{token}}`}</code>.
        </p>
      </div>

      <div className="mb-4">
        <KVEditor label="Query parameters" rows={form.queryParams} onChange={(v) => set('queryParams', v)} />
      </div>

      {form.method !== 'GET' && form.method !== 'DELETE' && form.method !== 'HEAD' && (
        <div className="mb-4">
          <label className="block text-[11px] font-medium text-muted-foreground mb-2">Request body</label>
          <div className="flex gap-2 mb-3">
            {(['template', 'free'] as const).map((mode) => (
              <button key={mode} type="button" onClick={() => set('bodyMode', mode)}
                className={`px-3 py-1.5 text-[12px] rounded-lg border transition-colors ${
                  form.bodyMode === mode
                    ? 'bg-accent text-white border-purple-600'
                    : 'bg-surface text-muted-foreground border-border-strong hover:border-gray-300'
                }`}>
                {mode === 'template' ? 'Template' : 'Free-form'}
              </button>
            ))}
          </div>

          {form.bodyMode === 'template' ? (
            <div>
              <p className="text-[11px] text-muted-foreground mb-2">
                Write the JSON body you want to send. Use <code className="font-mono bg-muted px-1 rounded">{`{{variable}}`}</code> for parts the LLM should fill in.
              </p>
              <textarea
                value={form.bodyTemplate}
                onChange={(e) => set('bodyTemplate', e.target.value)}
                rows={5}
                placeholder={`{\n  "channel": "{{channel}}",\n  "text": "{{message}}"\n}`}
                className="w-full text-[12px] font-mono px-3 py-2 border border-border-strong rounded-lg resize-y bg-surface"
              />
              {detectedVars.length > 0 && (
                <div className="mt-2 flex flex-wrap gap-1.5">
                  <span className="text-[10px] text-faint">LLM will provide:</span>
                  {detectedVars.map((v) => (
                    <span key={v} className="text-[10px] px-2 py-0.5 bg-accent/10 text-accent dark:text-accent-bright border border-accent/25 rounded-full font-mono">
                      {v}
                    </span>
                  ))}
                </div>
              )}
            </div>
          ) : (
            <div className="bg-muted border border-border-strong rounded-lg px-4 py-3">
              <p className="text-[12px] text-muted-foreground">
                The LLM constructs the entire body as a JSON object and passes it as <code className="font-mono bg-muted px-1 rounded">body</code>.
              </p>
            </div>
          )}
        </div>
      )}

      <div className="flex flex-wrap items-end gap-3 mb-4">
        <div>
          <label className="block text-[11px] font-medium text-muted-foreground mb-1">Risk level</label>
          <select value={form.riskLevel} onChange={(e) => set('riskLevel', e.target.value)}
            className="text-sm px-3 py-2 border border-border-strong rounded-lg bg-surface">
            {RISK_OPTIONS.map((r) => <option key={r}>{r}</option>)}
          </select>
        </div>
        <div>
          <label className="block text-[11px] font-medium text-muted-foreground mb-1">Timeout (ms)</label>
          <input type="number" value={form.timeoutMs} onChange={(e) => set('timeoutMs', Number(e.target.value))}
            className="text-sm px-3 py-2 border border-border-strong rounded-lg w-28" />
        </div>
        <label className="flex items-center gap-2 text-[12px] text-muted-foreground cursor-pointer pb-2">
          <input type="checkbox" checked={form.requiresApproval}
            onChange={(e) => set('requiresApproval', e.target.checked)} />
          Require approval before running
        </label>
      </div>

      <div className="flex gap-2">
        <button
          onClick={() => create.mutate()} disabled={create.isPending}
          className="px-4 py-2 bg-accent text-white text-[12px] rounded-lg disabled:opacity-50 font-medium">
          {create.isPending ? 'Adding…' : 'Add tool'}
        </button>
        <button onClick={onClose} className="px-3 py-2 text-[12px] text-muted-foreground hover:text-gray-800 dark:hover:text-gray-200">Cancel</button>
      </div>
    </div>
  )
}

// ─── code tool form ───────────────────────────────────────────────────────────

function CodeToolPanel({
  initial, existingId, onClose,
}: {
  initial?: Partial<CodeForm>
  existingId?: string
  onClose: () => void
}) {
  const [form, setForm] = useState<CodeForm>({ ...emptyCodeForm(), ...initial })
  const [error, setError] = useState('')
  const [testInput, setTestInput] = useState('{\n  "name": "world"\n}')
  const [testResult, setTestResult] = useState<string | null>(null)
  const [testing, setTesting] = useState(false)
  const queryClient = useQueryClient()

  const set = <K extends keyof CodeForm>(k: K, v: CodeForm[K]) => setForm((f) => ({ ...f, [k]: v }))

  const parseInputSchema = () => {
    try { return JSON.parse(form.inputSchema) }
    catch { return { type: 'object', properties: {} } }
  }

  const save = useMutation({
    mutationFn: () => {
      if (!form.name.trim()) throw new Error('Tool name is required')
      if (!form.code.trim()) throw new Error('Code is required')
      const payload = {
        name: form.name.trim(),
        description: form.description.trim() || `${form.name.trim()} (JavaScript)`,
        type: 'code',
        risk_level: form.riskLevel,
        requires_approval: form.requiresApproval,
        timeout_ms: form.timeoutMs,
        enabled: true,
        input_schema: parseInputSchema(),
        output_schema: { type: 'object' },
        config: { language: 'javascript', code: form.code },
      }
      return existingId ? toolsAPI.update(existingId, payload) : toolsAPI.create(payload)
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['tools'] }); onClose() },
    onError: (err: Error) => setError(err.message),
  })

  const runTest = async () => {
    setTesting(true)
    setTestResult(null)
    try {
      const inputObj = JSON.parse(testInput)
      const res = await toolsAPI.testCode(form.code, inputObj)
      setTestResult(res.error
        ? `Error: ${res.error}`
        : JSON.stringify(res.result, null, 2))
    } catch (e) {
      setTestResult(`Error: ${e instanceof Error ? e.message : String(e)}`)
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="border border-border-strong rounded-xl p-5 mb-6 bg-surface shadow-sm">
      <div className="flex items-center justify-between mb-4">
        <div>
          <p className="text-sm font-medium text-foreground">{existingId ? 'Edit code tool' : 'New code tool'}</p>
          <p className="text-[11px] text-faint mt-0.5">
            Write JavaScript that runs in a sandboxed Goja VM. Receives <code className="font-mono bg-muted px-1 rounded">input</code> and returns any JSON-serializable value.
          </p>
        </div>
        <button onClick={onClose}><X size={15} className="text-faint" /></button>
      </div>

      {error && <p className="text-[12px] text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-100 rounded px-3 py-2 mb-3">{error}</p>}

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-4">
        <div>
          <label className="block text-[11px] font-medium text-muted-foreground mb-1">Tool name *</label>
          <input value={form.name} onChange={(e) => set('name', e.target.value)}
            placeholder="e.g. calculate_discount" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg font-mono" />
        </div>
        <div>
          <label className="block text-[11px] font-medium text-muted-foreground mb-1">Description</label>
          <input value={form.description} onChange={(e) => set('description', e.target.value)}
            placeholder="What this tool does" className="w-full text-sm px-3 py-2 border border-border-strong rounded-lg" />
        </div>
      </div>

      {/* Code editor */}
      <div className="mb-4">
        <label className="block text-[11px] font-medium text-muted-foreground mb-1">JavaScript code *</label>
        <div className="rounded-lg overflow-hidden border border-border-strong">
          <CodeEditor
            value={form.code}
            height="220px"
            theme="dark"
            onChange={(v) => set('code', v)}
            basicSetup={{ lineNumbers: true, foldGutter: false }}
          />
        </div>
      </div>

      {/* Input schema */}
      <div className="mb-4">
        <label className="block text-[11px] font-medium text-muted-foreground mb-1">
          Input schema <span className="font-normal text-faint">(JSON Schema — describes what the LLM must provide)</span>
        </label>
        <textarea
          value={form.inputSchema}
          onChange={(e) => set('inputSchema', e.target.value)}
          rows={5}
          className="w-full text-[12px] font-mono px-3 py-2 border border-border-strong rounded-lg resize-y bg-surface"
        />
      </div>

      {/* Risk + settings */}
      <div className="flex flex-wrap items-end gap-3 mb-5">
        <div>
          <label className="block text-[11px] font-medium text-muted-foreground mb-1">Risk level</label>
          <select value={form.riskLevel} onChange={(e) => set('riskLevel', e.target.value)}
            className="text-sm px-3 py-2 border border-border-strong rounded-lg bg-surface">
            {RISK_OPTIONS.map((r) => <option key={r}>{r}</option>)}
          </select>
        </div>
        <div>
          <label className="block text-[11px] font-medium text-muted-foreground mb-1">Timeout (ms)</label>
          <input type="number" value={form.timeoutMs} onChange={(e) => set('timeoutMs', Number(e.target.value))}
            className="text-sm px-3 py-2 border border-border-strong rounded-lg w-28" />
        </div>
        <label className="flex items-center gap-2 text-[12px] text-muted-foreground cursor-pointer pb-2">
          <input type="checkbox" checked={form.requiresApproval}
            onChange={(e) => set('requiresApproval', e.target.checked)} />
          Require approval before running
        </label>
      </div>

      {/* Test panel */}
      <div className="border border-border rounded-lg p-4 mb-4 bg-muted">
        <p className="text-[11px] font-medium text-muted-foreground mb-2">Test this tool</p>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label className="block text-[10px] text-muted-foreground mb-1">Input (JSON)</label>
            <textarea
              value={testInput}
              onChange={(e) => setTestInput(e.target.value)}
              rows={4}
              className="w-full text-[12px] font-mono px-3 py-2 border border-border-strong rounded-lg bg-surface resize-none"
            />
          </div>
          <div>
            <label className="block text-[10px] text-muted-foreground mb-1">Result</label>
            <pre className={`h-[88px] text-[12px] font-mono px-3 py-2 border rounded-lg overflow-auto whitespace-pre-wrap ${
              testResult?.startsWith('Error:')
                ? 'bg-red-50 dark:bg-red-500/10 border-red-100 text-red-700 dark:text-red-300'
                : 'bg-surface border-border-strong text-foreground'
            }`}>
              {testResult ?? <span className="text-faint">— run test to see output —</span>}
            </pre>
          </div>
        </div>
        <button
          onClick={runTest}
          disabled={testing}
          className="mt-2 inline-flex items-center gap-1.5 px-3 py-1.5 bg-gray-800 hover:bg-gray-900 text-white text-[12px] rounded-lg disabled:opacity-50 transition-colors"
        >
          <Play size={11} /> {testing ? 'Running…' : 'Run test'}
        </button>
      </div>

      <div className="flex gap-2">
        <button
          onClick={() => save.mutate()} disabled={save.isPending}
          className="px-4 py-2 bg-accent text-white text-[12px] rounded-lg disabled:opacity-50 font-medium">
          {save.isPending ? 'Saving…' : existingId ? 'Save changes' : 'Create tool'}
        </button>
        <button onClick={onClose} className="px-3 py-2 text-[12px] text-muted-foreground hover:text-gray-800 dark:hover:text-gray-200">Cancel</button>
      </div>
    </div>
  )
}

// ─── section ─────────────────────────────────────────────────────────────────

function Section({
  title, icon, blurb, emptyBlurb, tools, action, onToggle, onDelete, onEdit,
}: {
  title: string; icon: React.ReactNode; blurb: string; emptyBlurb?: string
  tools: Tool[]; action?: React.ReactNode
  onToggle: (t: Tool) => void; onDelete: (t: Tool) => void; onEdit?: (t: Tool) => void
}) {
  return (
    <div className="mb-7">
      <div className="flex items-center justify-between mb-1">
        <div className="flex items-center gap-2">
          <span className="text-faint">{icon}</span>
          <p className="text-[13px] font-medium text-foreground">{title}</p>
          <span className="text-[11px] text-faint bg-muted px-1.5 py-0.5 rounded-full">{tools.length}</span>
        </div>
        {action}
      </div>
      <p className="text-[11px] text-faint ml-6 mb-2">{blurb}</p>
      {tools.length > 0 ? (
        <div className="bg-surface border border-border rounded-xl overflow-hidden">
          {tools.map((t) => (
            <ToolRow
              key={t.id}
              tool={t}
              onToggle={() => onToggle(t)}
              onDelete={() => onDelete(t)}
              onEdit={onEdit ? () => onEdit(t) : undefined}
            />
          ))}
        </div>
      ) : (
        <div className="border border-dashed border-border-strong rounded-xl py-6 text-center">
          <p className="text-[12px] text-faint">{emptyBlurb ?? `No ${title.toLowerCase()} yet.`}</p>
        </div>
      )}
    </div>
  )
}

// ─── page ─────────────────────────────────────────────────────────────────────

export default function ToolsPage() {
  const queryClient = useQueryClient()
  const [addingHTTP, setAddingHTTP] = useState(false)
  const [addingCode, setAddingCode] = useState(false)
  const [editingTool, setEditingTool] = useState<Tool | null>(null)
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

  const tools   = data?.data ?? []
  const native  = tools.filter((t) => t.type === 'native')
  const mcp     = tools.filter((t) => t.type === 'mcp')
  const http    = tools.filter((t) => t.type === 'http')
  const code    = tools.filter((t) => t.type === 'code')

  const getCodeFormInitial = (t: Tool): Partial<CodeForm> => {
    const cfg = t.config ? (() => { try { return JSON.parse(t.config as unknown as string) } catch { return null } })() : null
    return {
      name: t.name,
      description: t.description,
      code: cfg?.code ?? DEFAULT_CODE,
      inputSchema: t.input_schema ? JSON.stringify(
        typeof t.input_schema === 'string' ? JSON.parse(t.input_schema) : t.input_schema,
        null, 2
      ) : emptyCodeForm().inputSchema,
      riskLevel: t.risk_level,
      requiresApproval: t.requires_approval,
      timeoutMs: t.timeout_ms,
    }
  }

  return (
    <div className="p-4 sm:p-6 max-w-3xl">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-foreground">Tools</h1>
        <p className="text-[12px] text-faint mt-0.5">
          Tools let agents take actions beyond text — run code, call APIs, search the web.
        </p>
        <div className="flex flex-wrap items-center gap-2 text-[11px] mt-3">
          <span className="text-faint">Risk levels:</span>
          {([
            { level: 'low',      cls: 'bg-blue-50 dark:bg-blue-500/10 text-blue-700 dark:text-blue-300 border-blue-100' },
            { level: 'medium',   cls: 'bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300 border-amber-100' },
            { level: 'high',     cls: 'bg-orange-50 dark:bg-orange-500/10 text-orange-700 dark:text-orange-300 border-orange-100' },
            { level: 'critical', cls: 'bg-red-50 dark:bg-red-500/10 text-red-700 dark:text-red-300 border-red-100' },
          ] as const).map(({ level, cls }) => (
            <span key={level} className={`px-2 py-0.5 rounded-full border ${cls}`}>{level}</span>
          ))}
          <span className="text-faint ml-1">
            · tools marked <span className="text-amber-700 dark:text-amber-300 font-medium">approval</span> pause the agent until a human approves
          </span>
        </div>
      </div>

      {(error || actionError) && (
        <div className="text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg p-3 mb-4">
          {actionError || (error as Error).message}
        </div>
      )}

      {isLoading && <div className="py-12 text-center text-sm text-faint">Loading…</div>}

      {!isLoading && (
        <>
          {addingHTTP && <HTTPToolForm onClose={() => setAddingHTTP(false)} />}
          {addingCode && !editingTool && (
            <CodeToolPanel onClose={() => setAddingCode(false)} />
          )}
          {editingTool && (
            <CodeToolPanel
              existingId={editingTool.id}
              initial={getCodeFormInitial(editingTool)}
              onClose={() => setEditingTool(null)}
            />
          )}

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
              <Link href="/mcp-servers" className="inline-flex items-center gap-1 text-[11px] text-accent dark:text-accent-bright hover:underline">
                Manage MCP servers <ExternalLink size={10} />
              </Link>
            }
            onToggle={(t) => toggle.mutate(t)}
            onDelete={(t) => { if (confirm(`Delete ${t.name}?`)) remove.mutate(t.id) }}
          />

          <Section
            title="Code tools"
            icon={<Terminal size={14} />}
            blurb="Custom JavaScript that runs in a sandboxed Goja VM. Write any logic — math, data transformation, formatting — without needing an external endpoint."
            emptyBlurb="No code tools yet. Write one to add custom logic for agents."
            tools={code}
            action={
              !addingCode && !editingTool ? (
                <button
                  onClick={() => { setAddingCode(true); setAddingHTTP(false); setActionError('') }}
                  className="inline-flex items-center gap-1 text-[11px] text-accent dark:text-accent-bright hover:underline">
                  <Plus size={11} /> New code tool
                </button>
              ) : undefined
            }
            onToggle={(t) => toggle.mutate(t)}
            onDelete={(t) => { if (confirm(`Delete ${t.name}?`)) remove.mutate(t.id) }}
            onEdit={(t) => { setEditingTool(t); setAddingCode(false); setAddingHTTP(false) }}
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
                  onClick={() => { setAddingHTTP(true); setAddingCode(false); setEditingTool(null); setActionError('') }}
                  className="inline-flex items-center gap-1 text-[11px] text-accent dark:text-accent-bright hover:underline">
                  <Plus size={11} /> Add HTTP tool
                </button>
              ) : undefined
            }
            onToggle={(t) => toggle.mutate(t)}
            onDelete={(t) => { if (confirm(`Delete ${t.name}?`)) remove.mutate(t.id) }}
          />
        </>
      )}

      <div className="mt-6 p-4 rounded-lg bg-muted border border-border-strong">
        <p className="text-sm text-muted-foreground">
          Learn about native tools, HTTP tools, code tools, risk levels, and approval gates in the{' '}
          <Link href="/docs/what-is-a-tool" className="text-accent dark:text-accent-bright hover:underline">
            documentation
          </Link>.
        </p>
      </div>
    </div>
  )
}
