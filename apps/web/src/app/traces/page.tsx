'use client'

import Link from 'next/link'
import { useQuery } from '@tanstack/react-query'
import { GitCommitHorizontal } from 'lucide-react'
import { agentsAPI, runsAPI } from '@/lib/api'
import { formatTokens, relativeTime, statusColor } from '@/lib/utils'
import type { Agent, Run } from '@/types'

export default function TracesPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['runs'],
    queryFn: () => runsAPI.list() as Promise<{ data: Run[] }>,
  })
  const { data: agentData } = useQuery({
    queryKey: ['agents'],
    queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }>,
  })
  const runs = data?.data ?? []
  const names = Object.fromEntries((agentData?.data ?? []).map((agent) => [agent.id, agent.name]))

  return <div className="p-6">
    <div className="mb-5"><h1 className="text-base font-medium text-gray-900">Traces</h1><p className="text-[12px] text-gray-400 mt-0.5">Inspect execution steps, inputs, outputs, and errors for recorded runs</p></div>
    {error && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">{(error as Error).message}</div>}
    {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading traces…</div>}
    {!isLoading && !error && runs.length === 0 && <div className="border border-dashed border-gray-200 rounded-xl py-12 text-center"><GitCommitHorizontal className="mx-auto text-gray-300 mb-3" /><p className="text-sm text-gray-500">No traces recorded yet. Start a conversation run to create one.</p></div>}
    {runs.length > 0 && <div className="bg-white border border-gray-100 rounded-xl overflow-hidden">{runs.map((run) => <Link key={run.id} href={`/traces/${run.id}`} className="grid grid-cols-12 items-center px-4 py-3 border-b last:border-b-0 border-gray-50 hover:bg-gray-50">
      <div className="col-span-4"><p className="font-mono text-[11px] text-gray-700">{run.id}</p><p className="text-[10px] text-gray-400 mt-0.5">{relativeTime(run.started_at)}</p></div>
      <span className="col-span-3 text-[12px] font-medium text-gray-800">{names[run.agent_id] ?? run.agent_id.slice(0, 8)}</span>
      <span className="col-span-2"><span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(run.status)}`}>{run.status}</span></span>
      <span className="col-span-2 text-[11px] text-gray-500">{formatTokens(run.total_input_tokens + run.total_output_tokens)} tokens</span>
      <span className="col-span-1 text-[11px] text-purple-600 text-right">Inspect</span>
    </Link>)}</div>}
  </div>
}
