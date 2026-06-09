'use client'

import { useQuery } from '@tanstack/react-query'
import { BarChart2 } from 'lucide-react'
import { agentsAPI, runsAPI } from '@/lib/api'
import { formatCost, formatTokens } from '@/lib/utils'
import type { Agent, Run } from '@/types'

const GROUP_KEY = '__group_runs__'

export default function UsagePage() {
  const { data, isLoading, error } = useQuery({ queryKey: ['runs'], queryFn: () => runsAPI.list() as Promise<{ data: Run[] }> })
  const { data: agentData } = useQuery({ queryKey: ['agents'], queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }> })
  const runs = data?.data ?? []
  const names = Object.fromEntries((agentData?.data ?? []).map((agent: Agent) => [agent.id, agent.name]))
  const totalTokens = runs.reduce((sum, run) => sum + run.total_input_tokens + run.total_output_tokens, 0)
  const totalCost = runs.reduce((sum, run) => sum + run.cost_estimate, 0)
  const byAgent = Object.values(
    runs.reduce<Record<string, { id: string; tokens: number; cost: number; runs: number }>>((acc, run) => {
      const key = run.agent_id || GROUP_KEY
      const item = acc[key] ?? { id: key, tokens: 0, cost: 0, runs: 0 }
      item.tokens += run.total_input_tokens + run.total_output_tokens
      item.cost += run.cost_estimate
      item.runs += 1
      acc[key] = item
      return acc
    }, {})
  ).sort((a, b) => b.tokens - a.tokens)

  function agentLabel(id: string) {
    if (id === GROUP_KEY) return 'Group runs'
    return names[id] ?? id.slice(0, 8)
  }

  return (
    <div className="p-6 max-w-4xl">
      <h1 className="text-base font-medium text-gray-900 mb-4">Usage</h1>
      <div className="grid grid-cols-4 gap-3 mb-6">
        {[
          ['Total tokens', formatTokens(totalTokens)],
          ['Estimated cost', formatCost(totalCost)],
          ['Recorded runs', String(runs.length)],
          ['Average tokens / run', formatTokens(runs.length ? Math.round(totalTokens / runs.length) : 0)],
        ].map(([label, value]) => (
          <div key={label} className="bg-gray-50 rounded-lg p-3">
            <p className="text-[11px] text-gray-400 mb-1">{label}</p>
            <p className="text-xl font-medium text-gray-900">{value}</p>
          </div>
        ))}
      </div>
      {error && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3">{(error as Error).message}</div>}
      {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading usage…</div>}
      {!isLoading && !error && byAgent.length === 0 && (
        <div className="border border-dashed border-gray-200 rounded-xl py-12 text-center">
          <BarChart2 className="mx-auto text-gray-300 mb-3" />
          <p className="text-sm text-gray-500">Usage will appear after your first recorded run.</p>
        </div>
      )}
      {byAgent.length > 0 && (
        <div>
          <p className="text-[12px] font-medium text-gray-700 mb-2">Usage by agent</p>
          <div className="bg-white border border-gray-100 rounded-xl p-4 space-y-4">
            {byAgent.map((item) => (
              <div key={item.id}>
                <div className="flex justify-between text-[11px] text-gray-600 mb-1">
                  <span>{agentLabel(item.id)}</span>
                  <span className="text-gray-400">{item.runs} runs · {formatTokens(item.tokens)} · {formatCost(item.cost)}</span>
                </div>
                <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
                  <div className="h-full bg-purple-500 rounded-full" style={{ width: `${totalTokens ? item.tokens / totalTokens * 100 : 0}%` }} />
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
