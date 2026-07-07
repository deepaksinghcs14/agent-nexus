'use client'

import { useQuery } from '@tanstack/react-query'
import { BarChart2 } from 'lucide-react'
import { runsAPI } from '@/lib/api'
import { formatCost, formatTokens } from '@/lib/utils'

interface AgentStat {
  agent_id: string
  agent_name: string
  tokens: number
  cost: number
  runs: number
}

interface UsageStats {
  total_tokens: number
  total_cost: number
  total_runs: number
  by_agent: AgentStat[]
}

export default function UsagePage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['runs-stats'],
    queryFn: () => runsAPI.stats() as Promise<UsageStats>,
  })

  const totalTokens = data?.total_tokens ?? 0
  const totalCost = data?.total_cost ?? 0
  const totalRuns = data?.total_runs ?? 0
  const byAgent = data?.by_agent ?? []

  return (
    <div className="p-4 sm:p-6 max-w-4xl">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-foreground">Usage</h1>
        <p className="text-sm text-muted-foreground mt-0.5">Token consumption and cost across all runs</p>
      </div>
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
        {[
          ['Total tokens', formatTokens(totalTokens)],
          ['Estimated cost', formatCost(totalCost)],
          ['Recorded runs', String(totalRuns)],
          ['Average tokens / run', formatTokens(totalRuns ? Math.round(totalTokens / totalRuns) : 0)],
        ].map(([label, value]) => (
          <div key={label} className="bg-surface border border-border rounded-xl p-4">
            <p className="text-xs text-faint mb-1">{label}</p>
            <p className="text-2xl font-bold text-foreground">{value}</p>
          </div>
        ))}
      </div>
      {error && <div className="text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg p-3">{(error as Error).message}</div>}
      {isLoading && <div className="py-12 text-center text-sm text-faint">Loading usage…</div>}
      {!isLoading && !error && byAgent.length === 0 && (
        <div className="border border-dashed border-border-strong rounded-xl py-12 text-center">
          <BarChart2 className="mx-auto text-faint mb-3" />
          <p className="text-sm text-muted-foreground">Usage will appear after your first recorded run.</p>
        </div>
      )}
      {byAgent.length > 0 && (
        <div>
          <p className="text-[12px] font-medium text-foreground mb-2">Usage by agent</p>
          <div className="bg-surface border border-border rounded-xl p-4 space-y-4">
            {byAgent.map((item) => (
              <div key={item.agent_id}>
                <div className="flex justify-between text-[11px] text-muted-foreground mb-1">
                  <span>{item.agent_name}</span>
                  <span className="text-faint">{item.runs} runs · {formatTokens(item.tokens)} · {formatCost(item.cost)}</span>
                </div>
                <div className="h-1.5 bg-muted rounded-full overflow-hidden">
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
