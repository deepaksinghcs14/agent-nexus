'use client'

import Link from 'next/link'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, CheckCircle2, Clock, Server } from 'lucide-react'
import { adminAPI } from '@/lib/api'
import { relativeTime, statusColor } from '@/lib/utils'

type WaitingRun = {
  run_id: string
  workspace: string
  agent_name: string
  status: string
  wait_type: string
  waiting_since: string
}

type PipelineOverview = {
  runner_configured: boolean
  runner_reachable: boolean
  runner_executor: string
  waiting_runs: WaitingRun[]
  runs_24h: Record<string, number>
}

// Instance-wide pipeline health: is the runner up, what's parked and for how
// long, and how runs fared in the last day. The "anything stuck?" page.
export default function AdminClaudeCodePage() {
  const { data, isLoading } = useQuery({
    queryKey: ['admin-pipeline'],
    queryFn: () => adminAPI.pipeline() as Promise<PipelineOverview>,
    refetchInterval: 15000,
  })

  const runnerOk = !!data?.runner_reachable
  const stub = data?.runner_executor === 'stub'
  const waiting = data?.waiting_runs ?? []
  const stats = data?.runs_24h ?? {}
  const statOrder = ['success', 'running', 'session_wait', 'approval_wait', 'failed', 'cancelled']

  return (
    <div className="p-4 sm:p-6 max-w-5xl">
      <div className="mb-5">
        <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">Claude Code</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">
          Runner health and durable waits across all workspaces.
        </p>
      </div>

      {isLoading && <div className="py-12 text-center text-sm text-gray-400 dark:text-gray-500">Loading…</div>}

      {data && (
        <>
          {/* Runner */}
          <div className={`flex flex-wrap items-center gap-3 border rounded-xl px-4 py-3 mb-4 ${
            !data.runner_configured ? 'border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/60'
              : runnerOk ? (stub ? 'border-amber-200 bg-amber-50/60' : 'border-green-200 bg-green-50/50')
              : 'border-red-200 bg-red-50/60'
          }`}>
            <Server size={16} className={!data.runner_configured ? 'text-gray-400 dark:text-gray-500' : runnerOk ? (stub ? 'text-amber-600 dark:text-amber-300' : 'text-green-600 dark:text-green-300') : 'text-red-500'} />
            <div>
              <p className="text-sm font-medium text-gray-900 dark:text-gray-100">
                Runner{data.runner_executor ? ` — ${data.runner_executor} mode` : ''}
              </p>
              <p className="text-xs text-gray-500 dark:text-gray-400">
                {!data.runner_configured
                  ? 'RUNNER_URL is not set — repo sessions are disabled instance-wide'
                  : !runnerOk
                    ? 'Configured but NOT RESPONDING — in-flight sessions will be recovered as crashed'
                    : stub
                      ? 'Reachable · ⚠ stub mode: all sessions are simulated'
                      : 'Reachable · real Claude Code sessions enabled'}
              </p>
            </div>
          </div>

          {/* 24h outcomes */}
          <div className="grid grid-cols-3 sm:grid-cols-6 gap-2 mb-5">
            {statOrder.map((s) => (
              <div key={s} className="bg-gray-50 dark:bg-gray-800/60 rounded-lg p-2.5 text-center">
                <p className="text-lg font-semibold text-gray-800 dark:text-gray-200">{stats[s] ?? 0}</p>
                <span className={`text-[10px] px-1.5 py-0.5 rounded-full ${statusColor(s)}`}>{s}</span>
              </div>
            ))}
          </div>

          {/* Waiting runs */}
          <div className="bg-white dark:bg-gray-900 border border-gray-100 dark:border-gray-800 rounded-xl overflow-hidden">
            <div className="flex items-center gap-2 px-4 py-3 border-b border-gray-100 dark:border-gray-800">
              <Clock size={14} className="text-purple-500" />
              <p className="text-[13px] font-medium text-gray-700 dark:text-gray-300">
                Waiting runs ({waiting.length})
              </p>
              <span className="text-[11px] text-gray-400 dark:text-gray-500">approval + session waits, all workspaces, oldest first</span>
            </div>
            {waiting.length === 0 ? (
              <p className="flex items-center gap-2 px-4 py-6 text-sm text-gray-400 dark:text-gray-500">
                <CheckCircle2 size={14} className="text-green-500" /> Nothing is waiting — no parked runs anywhere.
              </p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-[12px] min-w-[640px]">
                  <thead>
                    <tr className="bg-gray-50 dark:bg-gray-800/60 border-b border-gray-100 dark:border-gray-800">
                      {['Run', 'Workspace', 'Agent', 'Status', 'Durable', 'Waiting'].map((hd) => (
                        <th key={hd} className="text-left px-4 py-2 text-[10px] font-medium text-gray-400 dark:text-gray-500 uppercase">{hd}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {waiting.map((r) => {
                      const stale = Date.now() - new Date(r.waiting_since).getTime() > 6 * 3600_000
                      return (
                        <tr key={r.run_id} className="border-b border-gray-50">
                          <td className="px-4 py-2">
                            <Link href={`/runs/${r.run_id}`} className="font-mono text-purple-600 dark:text-purple-300 hover:underline">
                              {r.run_id.slice(0, 8)}…
                            </Link>
                          </td>
                          <td className="px-4 py-2 text-gray-700 dark:text-gray-300">{r.workspace}</td>
                          <td className="px-4 py-2 text-gray-700 dark:text-gray-300">{r.agent_name || '—'}</td>
                          <td className="px-4 py-2">
                            <span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(r.status)}`}>{r.status}</span>
                          </td>
                          <td className="px-4 py-2 text-gray-500 dark:text-gray-400">{r.wait_type}</td>
                          <td className="px-4 py-2">
                            <span className={`inline-flex items-center gap-1 ${stale ? 'text-amber-700 dark:text-amber-300 font-medium' : 'text-gray-500 dark:text-gray-400'}`}>
                              {stale && <AlertTriangle size={11} />}
                              {relativeTime(r.waiting_since)}
                            </span>
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}
