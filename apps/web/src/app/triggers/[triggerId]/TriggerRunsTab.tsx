'use client'

import Link from 'next/link'
import { useQuery } from '@tanstack/react-query'
import { Activity, Zap } from 'lucide-react'
import { runsAPI } from '@/lib/api'
import { formatCost, formatTokens, relativeTime, statusColor } from '@/lib/utils'
import type { Run } from '@/types'

interface Props {
  triggerId: string
}

function duration(run: Run): string {
  if (!run.completed_at) return '—'
  const ms = new Date(run.completed_at).getTime() - new Date(run.started_at).getTime()
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

export function TriggerRunsTab({ triggerId }: Props) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['runs', 'trigger', triggerId],
    queryFn: () => runsAPI.list({ trigger_id: triggerId }) as Promise<{ data: Run[] }>,
  })

  const runs = data?.data ?? []

  if (isLoading) {
    return <div className="py-12 text-center text-sm text-gray-400 dark:text-gray-500">Loading runs…</div>
  }

  if (error) {
    return <div className="py-6 text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 rounded-lg px-4">{(error as Error).message}</div>
  }

  if (runs.length === 0) {
    return (
      <div className="border border-dashed border-gray-200 dark:border-gray-700 rounded-xl py-14 text-center">
        <div className="w-10 h-10 rounded-full bg-[#f1f0ff] flex items-center justify-center mx-auto mb-3">
          <Zap className="w-5 h-5 text-purple-600 dark:text-purple-300" />
        </div>
        <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">No runs yet</p>
        <p className="text-xs text-gray-400 dark:text-gray-500 max-w-xs mx-auto">
          POST to the webhook URL above to fire your first run. Each invocation will appear here.
        </p>
      </div>
    )
  }

  return (
    <div className="bg-white dark:bg-gray-900 border border-gray-100 dark:border-gray-800 rounded-xl overflow-hidden">
      <div className="overflow-x-auto"><table className="w-full text-[12px] min-w-[540px]">
        <thead>
          <tr className="bg-gray-50 dark:bg-gray-800/60 border-b border-gray-100 dark:border-gray-800">
            {['Run ID', 'Status', 'Duration', 'Tokens', 'Cost', 'Started', ''].map((h) => (
              <th key={h} className="text-left px-4 py-2 text-[10px] font-medium text-gray-400 dark:text-gray-500 uppercase">{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {runs.map((run) => (
            <tr key={run.id} className="border-b last:border-b-0 border-gray-50 hover:bg-gray-50/50">
              <td className="px-4 py-2.5 font-mono text-[11px] text-gray-500 dark:text-gray-400">{run.id.slice(0, 12)}</td>
              <td className="px-4 py-2.5">
                <span className={`px-2 py-0.5 rounded-full text-[10px] font-medium ${statusColor(run.status)}`}>
                  {run.status}
                </span>
              </td>
              <td className="px-4 py-2.5 text-gray-500 dark:text-gray-400">{duration(run)}</td>
              <td className="px-4 py-2.5 text-gray-500 dark:text-gray-400">{formatTokens(run.total_input_tokens + run.total_output_tokens)}</td>
              <td className="px-4 py-2.5 text-gray-500 dark:text-gray-400">{formatCost(run.cost_estimate)}</td>
              <td className="px-4 py-2.5 text-gray-400 dark:text-gray-500">{relativeTime(run.started_at)}</td>
              <td className="px-4 py-2.5">
                <Link
                  href={`/runs/${run.id}`}
                  className="text-[11px] px-2.5 py-1 border border-gray-200 dark:border-gray-700 rounded-md text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-800"
                >
                  Trace
                </Link>
              </td>
            </tr>
          ))}
        </tbody>
      </table></div>

      <div className="px-4 py-2.5 border-t border-gray-50 flex items-center gap-1.5 text-[11px] text-gray-400 dark:text-gray-500">
        <Activity className="w-3 h-3" />
        {runs.length} invocation{runs.length !== 1 ? 's' : ''}
      </div>
    </div>
  )
}
