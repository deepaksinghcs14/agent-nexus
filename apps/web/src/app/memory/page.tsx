'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Brain, Check, Trash2, X } from 'lucide-react'
import { agentsAPI, memoryAPI } from '@/lib/api'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable, Row, Cell } from '@/components/ui/DataTable'
import type { Agent, Memory } from '@/types'

const scopeColor = (scope: string) => scope === 'agent' ? 'bg-accent/10 text-accent dark:text-accent-bright' : scope === 'workspace' ? 'bg-info/10 text-info' : 'bg-muted text-muted-foreground'
const sourceColor = (source: string) => source === 'tool' ? 'bg-good/10 text-good' : 'bg-warn/10 text-warn'

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
      <button onClick={() => { if (memories.length && confirm('Delete all memories matching these filters?')) bulkRemove.mutate() }} disabled={!memories.length || bulkRemove.isPending} className="inline-flex items-center gap-1.5 px-3 py-1.5 text-[12px] text-crit border border-crit/30 rounded-lg disabled:opacity-40"><Trash2 className="w-3.5 h-3.5" /> Delete filtered</button>
    </div>
    {error && <div className="text-sm text-crit bg-crit/10 border border-crit/30 rounded-lg p-3 mb-4">{(error as Error).message}</div>}
    {isLoading && <div className="py-12 text-center text-sm text-faint">Loading memories…</div>}
    {!isLoading && !error && memories.length === 0 && <div className="border border-dashed border-border-strong rounded-xl py-12 text-center bg-surface"><Brain className="mx-auto text-faint mb-3" /><p className="text-sm text-muted-foreground">No {tab} memories match these filters.</p></div>}
    {!isLoading && !error && memories.length > 0 && (
    <DataTable columns={['Memory', 'Scope', 'Agent', 'Relevance', '']} minWidth={760}>
      {memories.map((memory) => {
        const score = memory.importance_score ?? memory.relevance_score ?? 0
        const filled = Math.round(score * 5)
        return (
          <Row key={memory.id}>
            <Cell className="!whitespace-normal">
              <div className="text-[12.5px] text-foreground leading-relaxed max-w-xl">{memory.content}</div>
              {memory.metadata?.reason && <div className="text-[11px] text-faint mt-0.5 max-w-xl line-clamp-1">{String(memory.metadata.reason)}</div>}
            </Cell>
            <Cell>
              <span className={`font-mono text-[10px] px-2 py-0.5 rounded-full ${scopeColor(memory.scope)}`}>{memory.scope}</span>
              <span className={`font-mono text-[10px] px-2 py-0.5 rounded-full ml-1 ${sourceColor(memory.save_source)}`}>{memory.save_source}</span>
            </Cell>
            <Cell><span className="text-[11px] text-muted-foreground">{memory.agent_id ? (names[memory.agent_id] ?? 'Agent') : '—'}</span></Cell>
            <Cell>
              <span className="inline-flex items-center gap-0.5" title={`importance ${score.toFixed(2)}`}>
                {Array.from({ length: 5 }).map((_, i) => <span key={i} className={`w-1.5 h-1.5 rounded-full ${i < filled ? 'bg-accent dark:bg-accent-bright' : 'bg-muted'}`} />)}
              </span>
            </Cell>
            <Cell className="text-right">
              <span className="inline-flex items-center gap-1">
                {tab === 'pending' && <>
                  <button onClick={() => approve.mutate(memory.id)} disabled={approve.isPending} className="inline-flex items-center gap-1 px-2 py-1 text-[11px] text-good border border-good/30 rounded-lg disabled:opacity-40"><Check className="w-3 h-3" /> Approve</button>
                  <button onClick={() => reject.mutate(memory.id)} disabled={reject.isPending} className="inline-flex items-center gap-1 px-2 py-1 text-[11px] text-muted-foreground border border-border-strong rounded-lg disabled:opacity-40"><X className="w-3 h-3" /> Reject</button>
                </>}
                <button onClick={() => { if (confirm('Delete this memory?')) remove.mutate(memory.id) }} className="p-1.5 text-faint hover:text-crit hover:bg-crit/10 rounded-lg"><Trash2 className="w-3.5 h-3.5" /></button>
              </span>
            </Cell>
          </Row>
        )
      })}
    </DataTable>
    )}
    <div className="mt-6 p-4 rounded-lg bg-muted border border-border-strong">
      <p className="text-sm text-muted-foreground">
        Learn how memory scopes and vector retrieval work in the{' '}
        <a href="/docs/what-is-an-agent" className="text-accent dark:text-accent-bright hover:underline">documentation</a>.
      </p>
    </div>
  </div>
}
