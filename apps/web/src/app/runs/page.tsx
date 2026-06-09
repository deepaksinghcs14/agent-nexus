'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, Square } from 'lucide-react'
import { agentsAPI, runsAPI } from '@/lib/api'
import { formatCost, formatTokens, relativeTime, statusColor } from '@/lib/utils'
import type { Agent, Run } from '@/types'

const ACTIVE = new Set(['pending', 'running', 'approval_wait'])

export default function RunsPage() {
  const [agent, setAgent] = useState('')
  const [status, setStatus] = useState('')
  const queryClient = useQueryClient()
  const params = new URLSearchParams()
  if (agent) params.set('agent_id', agent)
  if (status) params.set('status', status)

  const { data, isLoading, error } = useQuery({
    queryKey: ['runs', agent, status],
    queryFn: () => runsAPI.list(params.toString()) as Promise<{ data: Run[] }>,
  })
  const { data: agentData } = useQuery({
    queryKey: ['agents'],
    queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }>,
  })
  const stop = useMutation({
    mutationFn: (id: string) => runsAPI.cancel(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['runs'] }),
  })

  const runs = data?.data ?? []
  const agents = agentData?.data ?? []
  const agentNames = Object.fromEntries(agents.map((item) => [item.id, item.name]))
  const completed = runs.filter((run) => run.status === 'success' || run.status === 'failed')
  const successCount = runs.filter((run) => run.status === 'success').length
  const totalTokens = runs.reduce((sum, run) => sum + run.total_input_tokens + run.total_output_tokens, 0)
  const totalCost = runs.reduce((sum, run) => sum + run.cost_estimate, 0)

  function agentLabel(agentId: string | null | undefined) {
    if (!agentId) return 'Group run'
    return agentNames[agentId] ?? agentId.slice(0, 8)
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-base font-medium text-gray-900">Runs</h1>
        <div className="flex gap-2">
          <select value={agent} onChange={(e) => setAgent(e.target.value)} className="text-[12px] px-2.5 py-1.5 border border-gray-200 rounded-lg bg-white">
            <option value="">All agents</option>
            {agents.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
          <select value={status} onChange={(e) => setStatus(e.target.value)} className="text-[12px] px-2.5 py-1.5 border border-gray-200 rounded-lg bg-white">
            <option value="">All statuses</option>
            {['pending', 'running', 'success', 'failed', 'cancelled', 'approval_wait'].map((item) => <option key={item}>{item}</option>)}
          </select>
        </div>
      </div>

      <div className="grid grid-cols-4 gap-3 mb-5">
        {[
          { label: 'Total runs', value: runs.length },
          { label: 'Success rate', value: completed.length ? `${Math.round(successCount / completed.length * 100)}%` : '—' },
          { label: 'Total tokens', value: formatTokens(totalTokens) },
          { label: 'Estimated cost', value: formatCost(totalCost) },
        ].map((item) => <div key={item.label} className="bg-gray-50 rounded-lg p-3"><p className="text-[11px] text-gray-400 mb-1">{item.label}</p><p className="text-xl font-medium text-gray-900">{item.value}</p></div>)}
      </div>

      {error && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">{(error as Error).message}</div>}
      {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading runs…</div>}
      {!isLoading && !error && runs.length === 0 && (
        <div className="border border-dashed border-gray-200 rounded-xl py-12 text-center">
          <Activity className="mx-auto text-gray-300 mb-3" />
          <p className="text-sm text-gray-500">No runs match these filters.</p>
        </div>
      )}

      {runs.length > 0 && (
        <div className="bg-white border border-gray-100 rounded-xl overflow-x-auto">
          <table className="w-full text-[12px]">
            <thead>
              <tr className="bg-gray-50 border-b border-gray-100">
                {['Run ID', 'Agent', 'Status', 'Tokens', 'Cost', 'Started', ''].map((h) => (
                  <th key={h} className="text-left px-4 py-2 text-[10px] font-medium text-gray-400 uppercase">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {runs.map((run) => (
                <tr key={run.id} className="border-b last:border-b-0 border-gray-50">
                  <td className="px-4 py-2.5 font-mono text-[11px] text-gray-500">{run.id.slice(0, 12)}</td>
                  <td className="px-4 py-2.5 font-medium text-gray-900">{agentLabel(run.agent_id)}</td>
                  <td className="px-4 py-2.5">
                    <span className={`px-2 py-0.5 rounded-full text-[10px] font-medium ${statusColor(run.status)}`}>{run.status}</span>
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
      )}
    </div>
  )
}
