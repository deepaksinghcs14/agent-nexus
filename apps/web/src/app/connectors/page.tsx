'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Database, FileText, Plus, RefreshCw, Trash2, X } from 'lucide-react'
import { connectorsAPI } from '@/lib/api'
import { relativeTime, statusColor } from '@/lib/utils'
import type { Connector, ConnectorDocument, ConnectorSyncJob } from '@/types'

export default function ConnectorsPage() {
  const queryClient = useQueryClient()
  const [adding, setAdding] = useState(false)
  const [selected, setSelected] = useState('')
  const [form, setForm] = useState({ name: '', provider: 'filesystem', type: 'native', auth_type: 'none', config: '{}' })
  const [actionError, setActionError] = useState('')
  const { data, isLoading, error } = useQuery({ queryKey: ['connectors'], queryFn: () => connectorsAPI.list() as Promise<{ data: Connector[] }> })
  const connectors = data?.data ?? []
  const selectedConnector = connectors.find((connector) => connector.id === selected)
  const { data: docsData, isLoading: docsLoading, error: docsError } = useQuery({ queryKey: ['connector-docs', selected], queryFn: () => connectorsAPI.documents(selected) as Promise<{ data: ConnectorDocument[] }>, enabled: !!selected })
  const { data: jobsData } = useQuery({ queryKey: ['connector-jobs', selected], queryFn: () => connectorsAPI.syncJobs(selected) as Promise<{ data: ConnectorSyncJob[] }>, enabled: !!selected })
  const create = useMutation({
    mutationFn: () => connectorsAPI.create({ ...form, config: JSON.parse(form.config || '{}') }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['connectors'] }); setAdding(false); setForm({ name: '', provider: 'filesystem', type: 'native', auth_type: 'none', config: '{}' }) },
    onError: (err: Error) => setActionError(err.message),
  })
  const sync = useMutation({ mutationFn: (id: string) => connectorsAPI.sync(id), onSuccess: (_, id) => { queryClient.invalidateQueries({ queryKey: ['connectors'] }); queryClient.invalidateQueries({ queryKey: ['connector-jobs', id] }) }, onError: (err: Error) => setActionError(err.message) })
  const remove = useMutation({ mutationFn: (id: string) => connectorsAPI.delete(id), onSuccess: () => { setSelected(''); queryClient.invalidateQueries({ queryKey: ['connectors'] }) }, onError: (err: Error) => setActionError(err.message) })
  const docs = docsData?.data ?? []
  const jobs = jobsData?.data ?? []

  return <div className="p-6">
    <div className="flex items-center justify-between mb-4"><div><h1 className="text-base font-medium text-gray-900">Context connectors</h1><p className="text-[12px] text-gray-400 mt-0.5">Index documents for agent context retrieval</p></div><button onClick={() => { setAdding(true); setActionError('') }} className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-[12px] rounded-lg"><Plus className="w-3.5 h-3.5" /> Connect source</button></div>
    {(error || actionError) && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">{actionError || (error as Error).message}</div>}
    {adding && <div className="border border-gray-200 rounded-xl p-4 mb-5 bg-gray-50">
      <div className="flex justify-between mb-3"><p className="text-sm font-medium text-gray-800">Connect source</p><button onClick={() => setAdding(false)}><X className="w-4 h-4 text-gray-400" /></button></div>
      <div className="grid grid-cols-2 gap-3"><input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Connector name" className="text-sm px-3 py-2 border border-gray-200 rounded-lg" /><input value={form.provider} onChange={(e) => setForm({ ...form, provider: e.target.value })} placeholder="Provider" className="text-sm px-3 py-2 border border-gray-200 rounded-lg" /><select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })} className="text-sm px-3 py-2 border border-gray-200 rounded-lg bg-white"><option value="native">Native</option><option value="mcp">MCP</option></select><input value={form.auth_type} onChange={(e) => setForm({ ...form, auth_type: e.target.value })} placeholder="Auth type" className="text-sm px-3 py-2 border border-gray-200 rounded-lg" /></div>
      <textarea value={form.config} onChange={(e) => setForm({ ...form, config: e.target.value })} rows={3} className="mt-3 w-full text-xs font-mono px-3 py-2 border border-gray-200 rounded-lg" aria-label="Connector JSON configuration" />
      <button onClick={() => { try { JSON.parse(form.config || '{}'); create.mutate() } catch { setActionError('Connector config must be valid JSON') } }} disabled={!form.name.trim() || !form.provider.trim() || create.isPending} className="mt-3 px-3 py-1.5 bg-purple-600 text-white text-xs rounded-lg disabled:opacity-50">{create.isPending ? 'Connecting…' : 'Connect source'}</button>
    </div>}
    {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading connectors…</div>}
    {!isLoading && !error && connectors.length === 0 && <div className="border border-dashed border-gray-200 rounded-xl py-12 text-center"><Database className="mx-auto text-gray-300 mb-3" /><p className="text-sm text-gray-500">No context connectors configured.</p></div>}
    <div className="grid grid-cols-3 gap-3 mb-6">{connectors.map((connector) => <div key={connector.id} className={`bg-white border rounded-xl p-3 ${selected === connector.id ? 'border-purple-300' : 'border-gray-100'}`}><button onClick={() => setSelected(connector.id)} className="w-full text-left"><div className="flex items-center justify-between"><p className="text-[13px] font-medium text-gray-900">{connector.name}</p><span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(connector.status)}`}>{connector.status}</span></div><p className="text-[11px] text-gray-400 mt-1">{connector.provider} · {connector.type}</p></button><div className="flex gap-1.5 mt-3"><button onClick={() => sync.mutate(connector.id)} disabled={sync.isPending} className="inline-flex items-center gap-1 px-2 py-1 border border-gray-200 text-[11px] text-gray-600 rounded-md"><RefreshCw className={`w-3 h-3 ${sync.isPending ? 'animate-spin' : ''}`} /> Sync</button><button onClick={() => setSelected(connector.id)} className="inline-flex items-center gap-1 px-2 py-1 border border-gray-200 text-[11px] text-gray-600 rounded-md"><FileText className="w-3 h-3" /> Documents</button><button onClick={() => { if (confirm('Delete this connector?')) remove.mutate(connector.id) }} className="ml-auto p-1 text-gray-400 hover:text-red-500"><Trash2 className="w-3.5 h-3.5" /></button></div></div>)}</div>
    {selectedConnector && <div className="grid grid-cols-2 gap-5"><div><p className="text-[11px] font-medium text-gray-400 uppercase mb-2">Indexed documents</p>{docsError && <div className="text-xs text-red-600 bg-red-50 p-3 rounded-lg">{(docsError as Error).message}</div>}{docsLoading && <div className="text-xs text-gray-400 py-6 text-center">Loading documents…</div>}<div className="bg-white border border-gray-100 rounded-xl overflow-hidden">{docs.map((doc) => <div key={doc.id} className="p-3 border-b last:border-b-0 border-gray-50"><p className="text-[12px] font-medium text-gray-800">{doc.title}</p><p className="text-[10px] text-gray-400 mt-1">{doc.author || doc.source} · {doc.indexed_at ? `indexed ${relativeTime(doc.indexed_at)}` : 'not indexed'}</p>{doc.url && <a href={doc.url} target="_blank" rel="noreferrer" className="text-[10px] text-purple-600 hover:underline">Open source</a>}</div>)}{!docsLoading && !docsError && docs.length === 0 && <div className="p-8 text-center text-[12px] text-gray-400">No documents indexed.</div>}</div></div>
    <div><p className="text-[11px] font-medium text-gray-400 uppercase mb-2">Sync history</p><div className="bg-white border border-gray-100 rounded-xl overflow-hidden">{jobs.map((job) => <div key={job.id} className="p-3 border-b last:border-b-0 border-gray-50"><div className="flex items-center"><span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(job.status === 'completed' ? 'success' : job.status)}`}>{job.status}</span><span className="ml-auto text-[10px] text-gray-400">{relativeTime(job.created_at)}</span></div><p className="text-[11px] text-gray-500 mt-1">{job.documents_indexed} of {job.documents_found} documents indexed</p>{job.error_message && <p className="text-[10px] text-red-600 mt-1">{job.error_message}</p>}</div>)}{jobs.length === 0 && <div className="p-8 text-center text-[12px] text-gray-400">No sync jobs recorded.</div>}</div></div></div>}
  </div>
}
