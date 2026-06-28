'use client'

import { useState, useEffect, useRef, useCallback } from 'react'
import Link from 'next/link'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, Square, Zap, Loader2 } from 'lucide-react'
import { agentsAPI, runsAPI, webhookTriggersAPI, workflowsAPI } from '@/lib/api'
import { formatCost, formatTokens, relativeTime, statusColor } from '@/lib/utils'
import type { Agent, PaginatedRuns, Run, WebhookTrigger, Workflow } from '@/types'

const ACTIVE = new Set(['pending', 'running', 'approval_wait'])

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
    <div className="p-4 sm:p-6">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Runs</h1>
          <p className="text-sm text-gray-500 mt-0.5">Monitor all agent and workflow runs</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <select value={agent} onChange={(e) => setAgent(e.target.value)} className="text-[12px] px-2.5 py-1.5 border border-gray-200 rounded-lg bg-white">
            <option value="">All agents</option>
            {agents.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
          <select value={status} onChange={(e) => setStatus(e.target.value)} className="text-[12px] px-2.5 py-1.5 border border-gray-200 rounded-lg bg-white">
            <option value="">All statuses</option>
            {['pending', 'running', 'success', 'failed', 'cancelled', 'approval_wait'].map((s) => <option key={s}>{s}</option>)}
          </select>
        </div>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-5">
        {[
          { label: 'Loaded runs', value: runs.length + (hasMore ? '+' : '') },
          { label: 'Success rate', value: completed.length ? `${Math.round(successCount / completed.length * 100)}%` : '—' },
          { label: 'Total tokens', value: formatTokens(totalTokens) },
          { label: 'Estimated cost', value: formatCost(totalCost) },
        ].map((item) => <div key={item.label} className="bg-white border border-gray-100 rounded-xl p-4"><p className="text-[11px] text-gray-400 mb-1">{item.label}</p><p className="text-2xl font-bold text-gray-900">{item.value}</p></div>)}
      </div>

      {error && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">{error}</div>}
      {initialLoading && <div className="py-12 text-center text-sm text-gray-400">Loading runs…</div>}
      {!initialLoading && !error && runs.length === 0 && (
        <div className="border border-dashed border-gray-200 rounded-xl py-12 text-center">
          <Activity className="mx-auto text-gray-300 mb-3" />
          <p className="text-sm text-gray-500">No runs match these filters.</p>
        </div>
      )}

      {runs.length > 0 && (
        <div className="bg-white border border-gray-100 rounded-xl overflow-x-auto">
          <div className="min-w-[600px]">
          <table className="w-full text-[12px]">
            <thead>
              <tr className="bg-gray-50 border-b border-gray-100">
                {['Run ID', 'Agent / Workflow', 'Status', 'Source', 'Tokens', 'Cost', 'Started', ''].map((h) => (
                  <th key={h} className="text-left px-4 py-2 text-[10px] font-medium text-gray-400 uppercase">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {runs.map((run) => (
                <tr key={run.id} className="border-b last:border-b-0 border-gray-50 hover:bg-purple-50/30">
                  <td className="px-4 py-2.5 font-mono text-[11px] text-gray-500">{run.id.slice(0, 12)}</td>
                  <td className="px-4 py-2.5">
                    {(() => {
                      const { name, type } = runLabel(run)
                      return (
                        <div className="flex items-center gap-1.5">
                          <span className={`text-[9px] px-1.5 py-0.5 rounded font-medium flex-shrink-0 ${type === 'workflow' ? 'bg-purple-50 text-purple-600' : 'bg-indigo-50 text-indigo-600'}`}>
                            {type === 'workflow' ? 'WF' : 'AG'}
                          </span>
                          <span className="text-[12px] font-medium text-gray-900">{name}</span>
                        </div>
                      )
                    })()}
                  </td>
                  <td className="px-4 py-2.5">
                    <span className={`px-2 py-0.5 rounded-full text-[10px] font-medium ${statusColor(run.status)}`}>{run.status}</span>
                  </td>
                  <td className="px-4 py-2.5">
                    {run.trigger_id ? (
                      <span className="inline-flex items-center gap-1 text-[10px] px-2 py-0.5 rounded-full bg-rose-50 text-rose-600 font-medium">
                        <Zap className="w-2.5 h-2.5" /> Webhook
                      </span>
                    ) : (
                      <span className="text-[10px] text-gray-400">Manual</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-gray-500">{formatTokens(run.total_input_tokens + run.total_output_tokens)}</td>
                  <td className="px-4 py-2.5 text-gray-500">{formatCost(run.cost_estimate)}</td>
                  <td className="px-4 py-2.5 text-gray-400">{relativeTime(run.started_at)}</td>
                  <td className="px-4 py-2.5">
                    <div className="flex items-center gap-2">
                      <Link href={`/runs/${run.id}`} className="text-[11px] px-2.5 py-1 border border-gray-200 rounded-md text-gray-600 hover:bg-gray-50">
                        Trace
                      </Link>
                      {ACTIVE.has(run.status) && (
                        <button
                          onClick={() => stop.mutate(run.id)}
                          disabled={stop.isPending}
                          title="Stop run"
                          className="text-[11px] px-2.5 py-1 border border-red-200 rounded-md text-red-500 hover:bg-red-50 inline-flex items-center gap-1"
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
          <Loader2 className="w-4 h-4 text-gray-400 animate-spin" />
        </div>
      )}
      {!hasMore && runs.length > 0 && (
        <p className="text-center text-[11px] text-gray-300 py-4">All runs loaded</p>
      )}

      <div className="mt-4 p-4 rounded-lg bg-gray-50 border border-gray-200">
        <p className="text-sm text-gray-600">
          Learn about run statuses, approval gates, and polling in the{' '}
          <Link href="/docs/run-states" className="text-purple-600 hover:underline">
            documentation
          </Link>.
        </p>
      </div>
    </div>
  )
}
