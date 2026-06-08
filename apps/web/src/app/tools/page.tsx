'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2, Wrench, X } from 'lucide-react'
import { toolsAPI } from '@/lib/api'
import { riskColor } from '@/lib/utils'
import type { Tool } from '@/types'

const initialForm = { name: '', description: '', type: 'http', risk_level: 'low', requires_approval: false, timeout_ms: 30000, enabled: true, input_schema: '{}', output_schema: '{}' }

export default function ToolsPage() {
  const queryClient = useQueryClient()
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState(initialForm)
  const [actionError, setActionError] = useState('')
  const { data, isLoading, error } = useQuery({ queryKey: ['tools'], queryFn: () => toolsAPI.list() as Promise<{ data: Tool[] }> })
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['tools'] })
  const update = useMutation({ mutationFn: (tool: Tool) => toolsAPI.update(tool.id, { ...tool, enabled: !tool.enabled }), onSuccess: refresh, onError: (err: Error) => setActionError(err.message) })
  const create = useMutation({
    mutationFn: () => toolsAPI.create({ ...form, input_schema: JSON.parse(form.input_schema), output_schema: JSON.parse(form.output_schema) }),
    onSuccess: () => { refresh(); setAdding(false); setForm(initialForm) },
    onError: (err: Error) => setActionError(err.message),
  })
  const remove = useMutation({ mutationFn: (id: string) => toolsAPI.delete(id), onSuccess: refresh, onError: (err: Error) => setActionError(err.message) })
  const tools = data?.data ?? []

  return <div className="p-6">
    <div className="flex items-center justify-between mb-4"><div><h1 className="text-base font-medium text-gray-900">Tool registry</h1><p className="text-[12px] text-gray-400 mt-0.5">Tools available to agents in this workspace</p></div><button onClick={() => { setAdding(true); setActionError('') }} className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-[12px] rounded-lg"><Plus className="w-3.5 h-3.5" /> Add tool</button></div>
    {(error || actionError) && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">{actionError || (error as Error).message}</div>}
    {adding && <div className="border border-gray-200 rounded-xl p-4 mb-5 bg-gray-50">
      <div className="flex justify-between mb-3"><p className="text-sm font-medium text-gray-800">Add tool</p><button onClick={() => setAdding(false)}><X className="w-4 h-4 text-gray-400" /></button></div>
      <div className="grid grid-cols-2 gap-3"><input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Tool name" className="text-sm px-3 py-2 border border-gray-200 rounded-lg" /><input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} placeholder="Description" className="text-sm px-3 py-2 border border-gray-200 rounded-lg" /><select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })} className="text-sm px-3 py-2 border border-gray-200 rounded-lg bg-white"><option value="http">HTTP</option><option value="native">Native</option><option value="mcp">MCP</option></select><select value={form.risk_level} onChange={(e) => setForm({ ...form, risk_level: e.target.value })} className="text-sm px-3 py-2 border border-gray-200 rounded-lg bg-white">{['low', 'medium', 'high', 'critical'].map((risk) => <option key={risk}>{risk}</option>)}</select></div>
      <div className="grid grid-cols-2 gap-3 mt-3"><textarea value={form.input_schema} onChange={(e) => setForm({ ...form, input_schema: e.target.value })} rows={3} aria-label="Input schema" className="text-xs font-mono px-3 py-2 border border-gray-200 rounded-lg" /><textarea value={form.output_schema} onChange={(e) => setForm({ ...form, output_schema: e.target.value })} rows={3} aria-label="Output schema" className="text-xs font-mono px-3 py-2 border border-gray-200 rounded-lg" /></div>
      <label className="flex items-center gap-2 text-xs text-gray-600 mt-3"><input type="checkbox" checked={form.requires_approval} onChange={(e) => setForm({ ...form, requires_approval: e.target.checked })} /> Require approval</label>
      <button onClick={() => { try { JSON.parse(form.input_schema); JSON.parse(form.output_schema); create.mutate() } catch { setActionError('Input and output schemas must be valid JSON') } }} disabled={!form.name.trim() || create.isPending} className="mt-3 px-3 py-1.5 bg-purple-600 text-white text-xs rounded-lg disabled:opacity-50">{create.isPending ? 'Adding…' : 'Add tool'}</button>
    </div>}
    {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading tools…</div>}
    {!isLoading && !error && tools.length === 0 && <div className="border border-dashed border-gray-200 rounded-xl py-12 text-center"><Wrench className="mx-auto text-gray-300 mb-3" /><p className="text-sm text-gray-500">No tools registered.</p></div>}
    <div className="bg-white border border-gray-100 rounded-xl overflow-hidden">{tools.map((tool) => <div key={tool.id} className="flex items-center justify-between px-4 py-3 border-b last:border-b-0 border-gray-50"><div><p className="text-[13px] font-medium text-gray-900 font-mono">{tool.name}</p><p className="text-[11px] text-gray-500 mt-0.5">{tool.description}</p></div><div className="flex items-center gap-3"><span className={`text-[10px] px-2 py-0.5 rounded-full border ${riskColor(tool.risk_level)}`}>{tool.risk_level} risk</span>{tool.requires_approval && <span className="text-[10px] px-2 py-0.5 rounded-full bg-purple-50 text-purple-700">approval required</span>}<button onClick={() => update.mutate(tool)} disabled={update.isPending} aria-label={`${tool.enabled ? 'Disable' : 'Enable'} ${tool.name}`} className={`rounded-full relative ${tool.enabled ? 'bg-purple-600' : 'bg-gray-200'}`} style={{ width: 32, height: 18 }}><span className={`absolute top-0.5 w-3.5 h-3.5 bg-white rounded-full transition-all ${tool.enabled ? 'left-[14px]' : 'left-0.5'}`} /></button><button onClick={() => { if (confirm('Delete this tool?')) remove.mutate(tool.id) }} className="p-1 text-gray-400 hover:text-red-500"><Trash2 className="w-3.5 h-3.5" /></button></div></div>)}</div>
  </div>
}
