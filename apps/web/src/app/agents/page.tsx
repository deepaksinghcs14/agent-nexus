'use client'

import { useRef, useMemo, useState } from 'react'
import Link from 'next/link'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, MessageSquare, Pencil, Trash2, Bot, Download, Upload } from 'lucide-react'
import { agentsAPI } from '@/lib/api'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/ui/PageHeader'
import { cn } from '@/lib/utils'
import type { Agent } from '@/types'

const statusColors: Record<string, string> = {
  active: 'text-good border-good/40',
  paused: 'text-warn border-warn/40',
  archived: 'text-muted-foreground border-border',
}

export default function AgentsPage() {
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [importError, setImportError] = useState('')
  const fileInputRef = useRef<HTMLInputElement>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['agents'],
    queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }>,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => agentsAPI.delete(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  })

  const bulkDeleteMutation = useMutation({
    mutationFn: async (ids: string[]) => { await Promise.all(ids.map((id) => agentsAPI.delete(id))) },
    onSuccess: () => { setSelected(new Set()); queryClient.invalidateQueries({ queryKey: ['agents'] }) },
  })

  const importMutation = useMutation({
    mutationFn: (body: unknown) => agentsAPI.importAgent(body),
    onSuccess: () => { setImportError(''); queryClient.invalidateQueries({ queryKey: ['agents'] }) },
    onError: (err: Error) => setImportError(err.message),
  })

  function handleImportFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => {
      try { importMutation.mutate(JSON.parse(ev.target?.result as string)) }
      catch { setImportError('Invalid JSON file') }
    }
    reader.readAsText(file)
    e.target.value = ''
  }

  function exportAgent(agent: Agent) {
    const token = typeof window !== 'undefined' ? localStorage.getItem('access_token') : null
    fetch(agentsAPI.exportUrl(agent.id), { headers: token ? { Authorization: `Bearer ${token}` } : {} })
      .then(r => r.blob())
      .then(blob => {
        const a = document.createElement('a')
        a.href = URL.createObjectURL(blob)
        a.download = `agent_${agent.name.toLowerCase().replace(/\s+/g, '_')}.json`
        a.click()
        URL.revokeObjectURL(a.href)
      })
  }

  const agents = useMemo(() => data?.data ?? [], [data?.data])
  const visibleIds = useMemo(() => agents.filter((a) => !a.protected).map((a) => a.id), [agents])
  const selectedCount = selected.size
  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selected.has(id))

  function toggleSelected(id: string) {
    setSelected((prev) => { const next = new Set(prev); if (next.has(id)) next.delete(id); else next.add(id); return next })
  }
  function toggleAllVisible() {
    setSelected((prev) => {
      const next = new Set(prev)
      if (allVisibleSelected) visibleIds.forEach((id) => next.delete(id))
      else visibleIds.forEach((id) => next.add(id))
      return next
    })
  }
  function removeSelected() {
    const ids = Array.from(selected).filter((id) => visibleIds.includes(id))
    if (ids.length === 0) return
    if (confirm(`Delete ${ids.length} selected agent${ids.length !== 1 ? 's' : ''}?`)) bulkDeleteMutation.mutate(ids)
  }

  const iconBtn = 'p-1.5 rounded-lg text-muted-foreground transition-colors'

  return (
    <div className="p-4 sm:p-6 max-w-6xl">
      <PageHeader
        eyebrow="Build"
        title="Agents"
        subtitle={`${agents.length} agent${agents.length !== 1 ? 's' : ''} in this workspace`}
        actions={
          <>
            <input ref={fileInputRef} type="file" accept=".json" className="hidden" onChange={handleImportFile} />
            <button
              onClick={() => { setImportError(''); fileInputRef.current?.click() }}
              disabled={importMutation.isPending}
              className="inline-flex items-center gap-1.5 px-3 py-2 border border-border-strong text-muted-foreground text-[13px] rounded-[10px] hover:bg-muted disabled:opacity-60"
            >
              <Upload size={15} /> {importMutation.isPending ? 'Importing…' : 'Import'}
            </button>
            <Link href="/agents/new" className="inline-flex items-center gap-1.5 px-3.5 py-2 bg-gradient-to-br from-accent to-accent-ink text-white text-[13px] font-semibold rounded-[10px] shadow-card hover:opacity-95">
              <Plus size={15} /> New agent
            </Link>
          </>
        }
      />

      {importError && (
        <div className="mb-4 text-sm text-crit bg-crit/10 border border-crit/30 rounded-lg px-3 py-2">{importError}</div>
      )}

      {!isLoading && agents.length > 0 && (
        <div className="flex items-center justify-between mb-3 rounded-xl border border-border bg-surface px-3.5 py-2 shadow-card">
          <label className="flex items-center gap-2 text-[12px] text-muted-foreground">
            <input type="checkbox" checked={allVisibleSelected} onChange={toggleAllVisible} className="h-4 w-4 rounded border-border-strong text-accent focus:ring-accent" />
            Select all visible
          </label>
          <div className="flex items-center gap-2">
            <span className="text-[11px] text-faint tabular-nums">{selectedCount} selected</span>
            <button
              onClick={removeSelected}
              disabled={selectedCount === 0 || bulkDeleteMutation.isPending}
              className="inline-flex items-center gap-1.5 rounded-lg border border-crit/30 px-2.5 py-1 text-[11px] font-medium text-crit hover:bg-crit/10 disabled:cursor-not-allowed disabled:opacity-40"
            >
              <Trash2 size={13} />
              {bulkDeleteMutation.isPending ? 'Removing…' : 'Remove selected'}
            </button>
          </div>
        </div>
      )}

      {isLoading && (
        <div className="grid gap-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="border border-border rounded-xl p-4 bg-surface flex items-start gap-3">
              <Skeleton className="h-5 w-5 rounded flex-shrink-0" />
              <div className="flex-1 min-w-0"><Skeleton className="h-4 w-40 mb-2" /><Skeleton className="h-3 w-64 mb-2" /><Skeleton className="h-3 w-24" /></div>
            </div>
          ))}
        </div>
      )}

      {!isLoading && agents.length === 0 && (
        <div className="border border-dashed border-border-strong rounded-2xl p-12 text-center bg-surface">
          <Bot size={32} className="mx-auto text-muted-foreground mb-3" />
          <p className="text-muted-foreground text-sm">No agents yet.</p>
          <Link href="/agents/new" className="inline-flex mt-4 px-4 py-2 bg-accent text-white text-sm rounded-lg hover:bg-accent-hover">Create your first agent</Link>
        </div>
      )}

      <div className="grid gap-3">
        {agents.map((agent) => (
          <div key={agent.id} className={cn('border rounded-xl p-4 bg-surface shadow-card transition-all hover:border-border-strong', selected.has(agent.id) ? 'border-accent/50 ring-1 ring-accent/20' : 'border-border')}>
            <div className="flex items-start justify-between">
              <label className="mr-3 mt-0.5 flex h-5 w-5 items-center justify-center">
                {!agent.protected && (
                  <input type="checkbox" checked={selected.has(agent.id)} onChange={() => toggleSelected(agent.id)}
                    className="h-4 w-4 rounded border-border-strong text-accent focus:ring-accent" aria-label={`Select ${agent.name}`} />
                )}
              </label>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1 flex-wrap">
                  <span className="font-semibold text-foreground text-sm">{agent.name}</span>
                  <span className={cn('text-[10px] font-mono px-2 py-0.5 rounded-full border', statusColors[agent.status] ?? statusColors.active)}>{agent.status}</span>
                  <span className="text-[10px] font-mono px-2 py-0.5 rounded-full border border-border text-muted-foreground">{agent.provider}</span>
                  {agent.protected && <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-warn/15 text-warn">protected</span>}
                </div>
                {agent.description && <p className="text-xs text-muted-foreground truncate">{agent.description}</p>}
                <p className="text-xs text-faint font-mono mt-1">{agent.model}</p>
                {agent.name === 'Code Review Agent' && (
                  <p className="text-[11px] text-faint mt-1 italic">Used as a prompt template by repo sessions — not run as a chat agent.</p>
                )}
              </div>
              <div className="flex items-center gap-0.5 ml-4">
                <Link href={`/playground?agent=${agent.id}`} className={cn(iconBtn, 'hover:text-accent hover:bg-accent/10')} title="Chat"><MessageSquare size={15} /></Link>
                <Link href={`/agents/${agent.id}/edit`} className={cn(iconBtn, 'hover:text-foreground hover:bg-muted')} title="Edit"><Pencil size={15} /></Link>
                <button onClick={() => exportAgent(agent)} className={cn(iconBtn, 'hover:text-info hover:bg-info/10')} title="Export"><Download size={15} /></button>
                {!agent.protected && (
                  <button onClick={() => { if (confirm('Delete this agent?')) deleteMutation.mutate(agent.id) }} className={cn(iconBtn, 'hover:text-crit hover:bg-crit/10')} title="Delete"><Trash2 size={15} /></button>
                )}
              </div>
            </div>
          </div>
        ))}
      </div>

      <div className="mt-6 p-4 rounded-xl bg-muted border border-border">
        <p className="text-sm text-muted-foreground">
          Learn how to build and configure agents in the{' '}
          <Link href="/docs/what-is-an-agent" className="text-accent dark:text-accent-bright hover:underline">documentation</Link>.
        </p>
      </div>
    </div>
  )
}
