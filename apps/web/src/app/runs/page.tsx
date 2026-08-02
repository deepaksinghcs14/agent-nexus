'use client'

import { useState, useEffect, useRef, useCallback } from 'react'
import Link from 'next/link'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, Square, Zap, Loader2 } from 'lucide-react'
import { agentsAPI, runsAPI, webhookTriggersAPI, workflowsAPI } from '@/lib/api'
import { formatCost, formatTokens, relativeTime, statusColor } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/ui/PageHeader'
import type { Agent, PaginatedRuns, Run, WebhookTrigger, Workflow } from '@/types'

const ACTIVE = new Set(['pending', 'running', 'approval_wait', 'session_wait', 'user_input_wait'])

export default function RunsPage() {
  const [agent, setAgent] = useState('')
  const [status, setStatus] = useState('')
  const [runs, setRuns] = useState<Run[]>([])
  const [hasMore, setHasMore] = useState(false)
  const [nextCursor, setNextCursor] = useState('')
  const [loadingMore, setLoadingMore] = useState(false)
  const [initialLoading, setInitialLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const sentinelRef = useRef<HTMLDivElement>(null)
  const queryClient = useQueryClient()

  const { data: agentData } = useQuery({
    queryKey: ['agents'],
    queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }>,
  })
  const { data: workflowData } = useQuery({
    queryKey: ['workflows'],
    queryFn: () => workflowsAPI.list() as Promise<{ data: Workflow[] }>,
  })
  const { data: triggerData } = useQuery({
    queryKey: ['webhook-triggers'],
    queryFn: () => webhookTriggersAPI.list() as Promise<{ data: WebhookTrigger[] }>,
  })
  const agents = agentData?.data ?? []
  const agentNames = Object.fromEntries(agents.map((item) => [item.id, item.name]))
  const workflowNames = Object.fromEntries((workflowData?.data ?? []).map((w: Workflow) => [w.id, w.name]))
  const triggerMap = Object.fromEntries((triggerData?.data ?? []).map((t: WebhookTrigger) => [t.id, t]))

  const fetchPage = useCallback(async (cursor: string, reset: boolean) => {
    const params: Record<string, string> = {}
    if (agent) params.agent_id = agent
    if (status) params.status = status
    if (cursor) params.before = cursor
    try {
      const res = await runsAPI.listPage(params) as PaginatedRuns
      if (reset) {
        setRuns(res.data)
      } else {
        setRuns(prev => [...prev, ...res.data])
      }
      setHasMore(res.has_more)
      setNextCursor(res.next_cursor ?? '')
      setError(null)
    } catch (e) {
      setError((e as Error).message)
    }
  }, [agent, status])

  // Reset and reload when filters change
  useEffect(() => {
    setInitialLoading(true)
    setRuns([])
    setNextCursor('')
    setHasMore(false)
    fetchPage('', true).finally(() => setInitialLoading(false))
  }, [agent, status, fetchPage])

  // IntersectionObserver for infinite scroll
  useEffect(() => {
    if (!sentinelRef.current) return
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loadingMore) {
          setLoadingMore(true)
          fetchPage(nextCursor, false).finally(() => setLoadingMore(false))
        }
      },
      { rootMargin: '120px' }
    )
    observer.observe(sentinelRef.current)
    return () => observer.disconnect()
  }, [hasMore, loadingMore, nextCursor, fetchPage])

  const stop = useMutation({
    mutationFn: (id: string) => runsAPI.cancel(id),
    onSuccess: () => {
      fetchPage('', true)
      queryClient.invalidateQueries({ queryKey: ['runs'] })
    },
  })

  const completed = runs.filter((r) => r.status === 'success' || r.status === 'failed')
  const successCount = runs.filter((r) => r.status === 'success').length
  const totalTokens = runs.reduce((s, r) => s + r.total_input_tokens + r.total_output_tokens, 0)
  const totalCost = runs.reduce((s, r) => s + r.cost_estimate, 0)

  function runLabel(run: Run): { name: string; type: 'agent' | 'workflow' } {
    if (run.agent_id) {
      return { name: agentNames[run.agent_id] ?? run.agent_id.slice(0, 8), type: 'agent' }
    }
    if (run.trigger_id) {
      const trig = triggerMap[run.trigger_id]
      if (trig?.target_name) return { name: trig.target_name, type: 'workflow' }
      if (trig?.target_id && workflowNames[trig.target_id]) return { name: workflowNames[trig.target_id], type: 'workflow' }
    }
    return { name: 'Workflow run', type: 'workflow' }
  }

  return (
    <div className="p-4 sm:p-6 max-w-6xl">
      <PageHeader
        eyebrow="Observe"
        title="Runs & Traces"
        subtitle="Every agent and workflow execution, fully traced."
        actions={
          <>
            <select value={agent} onChange={(e) => setAgent(e.target.value)} className="text-[12px] px-2.5 py-2 border border-border-strong rounded-[10px] bg-surface">
              <option value="">All agents</option>
              {agents.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
            </select>
            <select value={status} onChange={(e) => setStatus(e.target.value)} className="text-[12px] px-2.5 py-2 border border-border-strong rounded-[10px] bg-surface">
              <option value="">All statuses</option>
              {['pending', 'running', 'success', 'failed', 'cancelled', 'approval_wait'].map((s) => <option key={s}>{s}</option>)}
            </select>
          </>
        }
      />

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-5">
        {[
          { label: 'Loaded runs', value: runs.length + (hasMore ? '+' : '') },
          { label: 'Success rate', value: completed.length ? `${Math.round(successCount / completed.length * 100)}%` : '—' },
          { label: 'Total tokens', value: formatTokens(totalTokens) },
          { label: 'Estimated cost', value: formatCost(totalCost) },
        ].map((item) => <div key={item.label} className="bg-surface border border-border rounded-xl p-4"><p className="text-[11px] text-faint mb-1">{item.label}</p><p className="text-2xl font-bold text-foreground">{item.value}</p></div>)}
      </div>

      {error && <div className="text-sm text-crit bg-crit/10 border border-crit/30 rounded-lg p-3 mb-4">{error}</div>}
      {initialLoading && (
        <div className="bg-surface border border-border rounded-xl overflow-x-auto">
          <div className="min-w-[600px] divide-y divide-border">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="flex items-center gap-4 px-4 py-3">
                <Skeleton className="h-3 w-20" />
                <Skeleton className="h-3 w-32" />
                <Skeleton className="h-4 w-16 rounded-full" />
                <Skeleton className="h-3 w-14" />
                <Skeleton className="h-3 w-10 ml-auto" />
                <Skeleton className="h-3 w-14" />
              </div>
            ))}
          </div>
        </div>
      )}
      {!initialLoading && !error && runs.length === 0 && (
        <div className="border border-dashed border-border-strong rounded-xl py-12 text-center">
          <Activity className="mx-auto text-faint mb-3" />
          <p className="text-sm text-muted-foreground">No runs match these filters.</p>
        </div>
      )}

      {runs.length > 0 && (
        <div className="bg-surface border border-border rounded-xl overflow-x-auto">
          <div className="min-w-[600px]">
          <table className="w-full text-[12px]">
            <thead>
              <tr className="bg-muted border-b border-border">
                {['Run ID', 'Agent / Workflow', 'Status', 'Source', 'Tokens', 'Cost', 'Started', ''].map((h) => (
                  <th key={h} className="text-left px-4 py-2 text-[10px] font-medium text-faint uppercase">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {runs.map((run) => (
                <tr key={run.id} className="border-b last:border-b-0 border-border hover:bg-accent/10">
                  <td className="px-4 py-2.5 font-mono text-[11px] text-muted-foreground">{run.id.slice(0, 12)}</td>
                  <td className="px-4 py-2.5">
                    {(() => {
                      const { name, type } = runLabel(run)
                      return (
                        <div className="flex items-center gap-1.5">
                          <span className={`text-[9px] px-1.5 py-0.5 rounded font-medium flex-shrink-0 ${type === 'workflow' ? 'bg-accent/10 text-accent dark:text-accent-bright' : 'bg-accent/10 dark:bg-info/10 text-info dark:text-info'}`}>
                            {type === 'workflow' ? 'WF' : 'AG'}
                          </span>
                          <span className="text-[12px] font-medium text-foreground">{name}</span>
                        </div>
                      )
                    })()}
                  </td>
                  <td className="px-4 py-2.5">
                    <span className={`px-2 py-0.5 rounded-full text-[10px] font-medium ${statusColor(run.status)}`}>{run.status}</span>
                  </td>
                  <td className="px-4 py-2.5">
                    {run.trigger_id ? (
                      <span className="inline-flex items-center gap-1 text-[10px] px-2 py-0.5 rounded-full bg-crit/10 dark:bg-crit/10 text-crit dark:text-crit font-medium">
                        <Zap className="w-2.5 h-2.5" /> Webhook
                      </span>
                    ) : (
                      <span className="text-[10px] text-faint">Manual</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-muted-foreground">{formatTokens(run.total_input_tokens + run.total_output_tokens)}</td>
                  <td className="px-4 py-2.5 text-muted-foreground">{formatCost(run.cost_estimate)}</td>
                  <td className="px-4 py-2.5 text-faint">{relativeTime(run.started_at)}</td>
                  <td className="px-4 py-2.5">
                    <div className="flex items-center gap-2">
                      <Link href={`/runs/${run.id}`} className="text-[11px] px-2.5 py-1 border border-border-strong rounded-md text-muted-foreground hover:bg-muted">
                        Trace
                      </Link>
                      {ACTIVE.has(run.status) && (
                        <button
                          onClick={() => stop.mutate(run.id)}
                          disabled={stop.isPending}
                          title="Stop run"
                          className="text-[11px] px-2.5 py-1 border border-crit/30 rounded-md text-crit hover:bg-crit/10 inline-flex items-center gap-1"
                        >
                          <Square className="w-3 h-3" /> Stop
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          </div>
        </div>
      )}

      {/* Infinite scroll sentinel */}
      <div ref={sentinelRef} className="h-4" />
      {loadingMore && (
        <div className="py-4 flex justify-center">
          <Loader2 className="w-4 h-4 text-faint animate-spin" />
        </div>
      )}
      {!hasMore && runs.length > 0 && (
        <p className="text-center text-[11px] text-faint py-4">All runs loaded</p>
      )}

      <div className="mt-4 p-4 rounded-lg bg-muted border border-border-strong">
        <p className="text-sm text-muted-foreground">
          Learn about run statuses, approval gates, and polling in the{' '}
          <Link href="/docs/run-states" className="text-accent dark:text-accent-bright hover:underline">
            documentation
          </Link>.
        </p>
      </div>
    </div>
  )
}
