'use client'

import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Plug, Plus, RefreshCw, Trash2, X } from 'lucide-react'
import { mcpAPI } from '@/lib/api'
import { relativeTime, riskColor } from '@/lib/utils'
import type { MCPServer, MCPTool } from '@/types'

export default function MCPServersPage() {
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState('')
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState({ name: '', url: '', transport: 'http' })
  const [actionError, setActionError] = useState('')
  const { data, isLoading, error } = useQuery({ queryKey: ['mcp-servers'], queryFn: () => mcpAPI.list() as Promise<{ data: MCPServer[] }> })
  const servers = data?.data ?? []
  useEffect(() => { if (!selected && servers[0]) setSelected(servers[0].id) }, [selected, servers])
  const selectedServer = servers.find((server) => server.id === selected)
  const { data: toolsData, isLoading: toolsLoading, error: toolsError } = useQuery({
    queryKey: ['mcp-tools', selected],
    queryFn: () => mcpAPI.tools(selected) as Promise<{ data: MCPTool[] }>,
    enabled: !!selected,
  })
  const create = useMutation({
    mutationFn: () => mcpAPI.create(form),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['mcp-servers'] }); setAdding(false); setForm({ name: '', url: '', transport: 'http' }) },
    onError: (err: Error) => setActionError(err.message),
  })
  const sync = useMutation({ mutationFn: (id: string) => mcpAPI.sync(id), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['mcp-servers'] }), onError: (err: Error) => setActionError(err.message) })
  const remove = useMutation({ mutationFn: (id: string) => mcpAPI.delete(id), onSuccess: () => { setSelected(''); queryClient.invalidateQueries({ queryKey: ['mcp-servers'] }) }, onError: (err: Error) => setActionError(err.message) })
  const tools = toolsData?.data ?? []

  return <div className="p-6">
    <div className="flex items-center justify-between mb-4"><div><h1 className="text-base font-medium text-gray-900">MCP Servers</h1><p className="text-[12px] text-gray-400 mt-0.5">Connect and discover tools from MCP servers</p></div><button onClick={() => { setAdding(true); setActionError('') }} className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-[12px] rounded-lg"><Plus className="w-3.5 h-3.5" /> Add Server</button></div>
    {(error || actionError) && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">{actionError || (error as Error).message}</div>}
    {adding && <div className="border border-gray-200 rounded-xl p-4 mb-5 bg-gray-50">
      <div className="flex justify-between mb-3"><p className="text-sm font-medium text-gray-800">Add MCP server</p><button onClick={() => setAdding(false)}><X className="w-4 h-4 text-gray-400" /></button></div>
      <div className="grid grid-cols-3 gap-3"><input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Server name" className="text-sm px-3 py-2 border border-gray-200 rounded-lg" /><input value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="https://server.example/mcp" className="text-sm px-3 py-2 border border-gray-200 rounded-lg" /><select value={form.transport} onChange={(e) => setForm({ ...form, transport: e.target.value })} className="text-sm px-3 py-2 border border-gray-200 rounded-lg bg-white"><option value="http">HTTP</option><option value="stdio">stdio</option></select></div>
      <button onClick={() => create.mutate()} disabled={!form.name.trim() || !form.url.trim() || create.isPending} className="mt-3 px-3 py-1.5 bg-purple-600 text-white text-xs rounded-lg disabled:opacity-50">{create.isPending ? 'Adding…' : 'Add server'}</button>
    </div>}
    {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading servers…</div>}
    {!isLoading && !error && servers.length === 0 && <div className="border border-dashed border-gray-200 rounded-xl py-12 text-center"><Plug className="mx-auto text-gray-300 mb-3" /><p className="text-sm text-gray-500">No MCP servers connected.</p></div>}
    {servers.length > 0 && <div className="grid grid-cols-3 gap-5">
      <div className="bg-white border border-gray-100 rounded-xl overflow-hidden h-fit">{servers.map((server) => <button key={server.id} onClick={() => setSelected(server.id)} className={`w-full flex items-center gap-3 px-3 py-3 text-left border-b last:border-b-0 border-gray-50 ${selected === server.id ? 'bg-purple-50' : 'hover:bg-gray-50'}`}><div className={`w-2 h-2 rounded-full ${server.status === 'connected' ? 'bg-green-500' : server.status === 'error' ? 'bg-red-400' : 'bg-gray-300'}`} /><div className="min-w-0 flex-1"><p className="text-[12px] font-medium text-gray-800 truncate">{server.name}</p><p className="text-[10px] text-gray-400 truncate">{server.url}</p></div>{server.status === 'error' && <AlertTriangle className="w-3.5 h-3.5 text-red-400" />}</button>)}</div>
      <div className="col-span-2">{selectedServer && <><div className="flex items-center justify-between mb-3"><div><p className="text-[13px] font-medium text-gray-900">{selectedServer.name}</p><p className="text-[11px] text-gray-400">{selectedServer.transport} · {selectedServer.tools_synced_at ? `synced ${relativeTime(selectedServer.tools_synced_at)}` : 'not synced'}</p></div><div className="flex gap-2"><button onClick={() => sync.mutate(selectedServer.id)} disabled={sync.isPending} className="inline-flex items-center gap-1 px-2.5 py-1.5 border border-gray-200 text-[12px] text-gray-600 rounded-lg"><RefreshCw className={`w-3 h-3 ${sync.isPending ? 'animate-spin' : ''}`} /> Sync</button><button onClick={() => { if (confirm('Delete this server?')) remove.mutate(selectedServer.id) }} className="p-1.5 border border-red-200 rounded-lg text-red-500"><Trash2 className="w-3.5 h-3.5" /></button></div></div>
      {toolsError && <div className="text-xs text-red-600 bg-red-50 p-3 rounded-lg">{(toolsError as Error).message}</div>}{toolsLoading && <div className="text-xs text-gray-400 py-6 text-center">Loading tools…</div>}<div className="bg-white border border-gray-100 rounded-xl overflow-hidden">{tools.map((tool) => <div key={tool.id} className="grid grid-cols-12 gap-2 items-center px-4 py-3 border-b last:border-b-0 border-gray-50"><span className="col-span-5 text-[12px] font-medium text-gray-800 font-mono truncate">{tool.name}</span><span className="col-span-5 text-[11px] text-gray-500 truncate">{tool.description}</span><span className={`col-span-2 text-[10px] px-2 py-0.5 rounded-full border ${riskColor(tool.risk_level)}`}>{tool.risk_level}</span></div>)}{!toolsLoading && !toolsError && tools.length === 0 && <div className="p-8 text-center text-[12px] text-gray-400">No tools discovered. Sync the server to discover tools.</div>}</div></>}</div>
    </div>}
  </div>
}
