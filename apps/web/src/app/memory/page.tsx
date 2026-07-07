'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Brain, Check, Trash2, X } from 'lucide-react'
import { agentsAPI, memoryAPI } from '@/lib/api'
import { PageHeader } from '@/components/ui/PageHeader'
import { relativeTime } from '@/lib/utils'
import type { Agent, Memory } from '@/types'

const scopeColor = (scope: string) => scope === 'agent' ? 'bg-accent/10 text-accent dark:text-accent-bright' : scope === 'workspace' ? 'bg-blue-50 dark:bg-blue-500/10 text-blue-700 dark:text-blue-300' : 'bg-muted text-muted-foreground'
const sourceColor = (source: string) => source === 'tool' ? 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : 'bg-amber-50 dark:bg-amber-500/10 text-amber-800 dark:text-amber-300'

export default function MemoryPage() {
  const queryClient = useQueryClient()
  const [tab, setTab] = useState<'active' | 'pending'>('active')
  const [search, setSearch] = useState('')
  const [agent, setAgent] = useState('')
  const [scope, setScope] = useState('')
  const [source, setSource] = useState('')
  const params = new URLSearchParams()
  params.set('status', tab)
  if (search) params.set('q', search)
  if (agent) params.set('agent_id', agent)
  if (scope) params.set('scope', scope)
  if (source) params.set('source', source)

  const { data, isLoading, error } = useQuery({
    queryKey: ['memory', tab, search, agent, scope, source],
    queryFn: () => memoryAPI.list(params.toString()) as Promise<{ data: Memory[] }>,
  })
  const { data: agentData } = useQuery({ queryKey: ['agents'], queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }> })
  const remove = useMutation({ mutationFn: (id: string) => memoryAPI.delete(id), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['memory'] }) })
  const approve = useMutation({ mutationFn: (id: string) => memoryAPI.approve(id), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['memory'] }) })
  const reject = useMutation({ mutationFn: (id: string) => memoryAPI.reject(id), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['memory'] }) })
  const bulkRemove = useMutation({ mutationFn: () => memoryAPI.bulkDelete(params.toString()), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['memory'] }) })

  const memories = data?.data ?? []
  const agents = agentData?.data ?? []
  const names = Object.fromEntries(agents.map((item) => [item.id, item.name]))

  return <div className="p-4 sm:p-6 max-w-6xl">
    <PageHeader eyebrow="Observe" title="Memory" subtitle="Stored facts your agents can recall across conversations." />
    <div className="flex flex-wrap items-start gap-3 mb-4">
      <div className="flex flex-wrap gap-2 flex-1">
        <div className="inline-flex border border-border-strong rounded-lg overflow-hidden bg-surface">
          <button onClick={() => setTab('active')} className={`px-3 py-1.5 text-[12px] ${tab === 'active' ? 'bg-accent text-white' : 'text-muted-foreground hover:bg-muted'}`}>Active</button>
          <button onClick={() => setTab('pending')} className={`px-3 py-1.5 text-[12px] ${tab === 'pending' ? 'bg-accent text-white' : 'text-muted-foreground hover:bg-muted'}`}>Pending</button>
        </div>
        <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search memories…" className="text-[12px] px-3 py-1.5 border border-border-strong rounded-lg w-full sm:w-52" />
        <select value={agent} onChange={(e) => setAgent(e.target.value)} className="text-[12px] px-2.5 py-1.5 border border-border-strong rounded-lg bg-surface"><option value="">All agents</option>{agents.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>
        <select value={scope} onChange={(e) => setScope(e.target.value)} className="text-[12px] px-2.5 py-1.5 border border-border-strong rounded-lg bg-surface"><option value="">All scopes</option><option>agent</option><option>workspace</option><option>conversation</option></select>
        <select value={source} onChange={(e) => setSource(e.target.value)} className="text-[12px] px-2.5 py-1.5 border border-border-strong rounded-lg bg-surface"><option value="">All sources</option><option value="tool">tool</option><option value="extractor">extractor</option></select>
      </div>
      <button onClick={() => { if (memories.length && confirm('Delete all memories matching these filters?')) bulkRemove.mutate() }} disabled={!memories.length || bulkRemove.isPending} className="inline-flex items-center gap-1.5 px-3 py-1.5 text-[12px] text-red-600 dark:text-red-300 border border-red-200 rounded-lg disabled:opacity-40"><Trash2 className="w-3.5 h-3.5" /> Delete filtered</button>
    </div>
    {error && <div className="text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg p-3 mb-4">{(error as Error).message}</div>}
    {isLoading && <div className="py-12 text-center text-sm text-faint">Loading memories…</div>}
    {!isLoading && !error && memories.length === 0 && <div className="border border-dashed border-border-strong rounded-xl py-12 text-center"><Brain className="mx-auto text-faint mb-3" /><p className="text-sm text-muted-foreground">No {tab} memories match these filters.</p></div>}
    <div className="bg-surface border border-border rounded-xl overflow-hidden">{memories.map((memory) => <div key={memory.id} className="p-4 border-b last:border-b-0 border-border">
      <div className="flex items-center gap-2 mb-1.5"><span className={`text-[10px] font-medium px-2 py-0.5 rounded-full ${scopeColor(memory.scope)}`}>{memory.scope}</span><span className={`text-[10px] font-medium px-2 py-0.5 rounded-full ${sourceColor(memory.save_source)}`}>{memory.save_source}</span>{memory.agent_id && <span className="text-[10px] px-2 py-0.5 rounded-full bg-muted text-muted-foreground">{names[memory.agent_id] ?? 'Agent'}</span>}<span className="ml-auto text-[10px] text-faint">{relativeTime(memory.created_at)}</span></div>
      <p className="text-[12px] text-foreground leading-relaxed">{memory.content}</p>
      {memory.metadata?.reason && <p className="mt-1 text-[11px] text-muted-foreground">{String(memory.metadata.reason)}</p>}
      <div className="flex items-center mt-2 gap-2">{(() => {
                const score = memory.importance_score ?? memory.relevance_score ?? 0
                const filled = Math.round(score * 5)
                return (
                  <span className="flex items-center gap-0.5" title={`importance ${score.toFixed(2)}`}>
                    {Array.from({length: 5}).map((_, i) => (
                      <span key={i} className={`w-1.5 h-1.5 rounded-full ${i < filled ? 'bg-accent' : 'bg-muted'}`} />
                    ))}
                  </span>
                )
              })()}{tab === 'pending' && <><button onClick={() => approve.mutate(memory.id)} disabled={approve.isPending} className="inline-flex items-center gap-1 px-2 py-1 text-[11px] text-emerald-700 dark:text-emerald-300 border border-emerald-200 rounded-lg disabled:opacity-40"><Check className="w-3 h-3" /> Approve</button><button onClick={() => reject.mutate(memory.id)} disabled={reject.isPending} className="inline-flex items-center gap-1 px-2 py-1 text-[11px] text-muted-foreground border border-border-strong rounded-lg disabled:opacity-40"><X className="w-3 h-3" /> Reject</button></>}<button onClick={() => { if (confirm('Delete this memory?')) remove.mutate(memory.id) }} className="ml-auto p-1 text-faint hover:text-red-500"><Trash2 className="w-3.5 h-3.5" /></button></div>
    </div>)}</div>
    <div className="mt-6 p-4 rounded-lg bg-muted border border-border-strong">
      <p className="text-sm text-muted-foreground">
        Learn how memory scopes and vector retrieval work in the{' '}
        <a href="/docs/what-is-an-agent" className="text-accent dark:text-accent-bright hover:underline">documentation</a>.
      </p>
    </div>
  </div>
}
