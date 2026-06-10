'use client'

import Link from 'next/link'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Ban } from 'lucide-react'
import { runsAPI } from '@/lib/api'
import { formatCost, formatTokens, statusColor } from '@/lib/utils'
import type { Run, RunStep } from '@/types'
import { TraceGraph, type RunWithWorkflow } from './TraceGraph'

type RunDetail = Run & { steps?: RunStep[]; workflow_id?: string }

export default function RunDetailPage({ params }: { params: { runId: string } }) {
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useQuery({
    queryKey: ['run', params.runId],
    queryFn: () => runsAPI.get(params.runId) as Promise<RunDetail | { run: Run; steps: RunStep[]; workflow_id?: string }>,
  })
  const cancel = useMutation({
    mutationFn: () => runsAPI.cancel(params.runId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['run', params.runId] }),
  })

  const detail: RunWithWorkflow | undefined = data
    ? 'run' in data
      ? {
          ...(data as { run: Run; steps: RunStep[]; workflow_id?: string }).run,
          steps: (data as { run: Run; steps: RunStep[] }).steps,
          workflow_id: (data as { workflow_id?: string }).workflow_id,
        }
      : (data as RunDetail)
    : undefined
  const steps = detail?.steps ?? []

  return (
    <div className="p-6 max-w-6xl">
      <div className="flex items-center justify-between mb-5">
        <Link href="/runs" className="text-[12px] text-gray-500 hover:text-gray-700 inline-flex items-center gap-1">
          <ArrowLeft className="w-3 h-3" /> Runs
        </Link>
        {detail && ['pending', 'running', 'approval_wait'].includes(detail.status) && (
          <button
            onClick={() => cancel.mutate()}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 border border-red-200 text-red-600 text-[12px] rounded-lg"
          >
            <Ban className="w-3.5 h-3.5" /> Cancel run
          </button>
        )}
      </div>

      {error && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3">{(error as Error).message}</div>
      )}
      {isLoading && (
        <div className="py-12 text-center text-sm text-gray-400">Loading trace…</div>
      )}

      {detail && (
        <>
          {/* Metrics row */}
          <div className="grid grid-cols-6 gap-3 mb-6">
            {[
              ['Status', detail.status],
              ['Input tokens', formatTokens(detail.total_input_tokens)],
              ['Output tokens', formatTokens(detail.total_output_tokens)],
              ['Cost', formatCost(detail.cost_estimate)],
              ['Started', new Date(detail.started_at).toLocaleString()],
              ['Completed', detail.completed_at ? new Date(detail.completed_at).toLocaleString() : '—'],
            ].map(([label, value]) => (
              <div key={label} className="bg-gray-50 rounded-lg p-2.5">
                <p className="text-[10px] text-gray-400 mb-1">{label}</p>
                {label === 'Status' ? (
                  <span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(value)}`}>{value}</span>
                ) : (
                  <p className="text-[11px] font-medium text-gray-800 break-words">{value}</p>
                )}
              </div>
            ))}
          </div>

          {/* Input / Output + Trace Graph */}
          <div className="grid grid-cols-2 gap-5">
            <div>
              <p className="text-[12px] font-medium text-gray-700 mb-2">Input</p>
              <div className="bg-gray-50 border border-gray-100 rounded-lg p-3 text-[12px] text-gray-600 whitespace-pre-wrap">
                {detail.input || 'No input recorded.'}
              </div>
              <p className="text-[12px] font-medium text-gray-700 mt-4 mb-2">Output</p>
              <div className="bg-gray-50 border border-gray-100 rounded-lg p-3 text-[12px] text-gray-600 whitespace-pre-wrap">
                {detail.output || detail.error_message || 'No output recorded.'}
              </div>
            </div>

            <div>
              <p className="text-[12px] font-medium text-gray-700 mb-2">
                Trace
                {steps.length > 0 && (
                  <span className="ml-1.5 text-[10px] text-gray-400 font-normal">
                    {steps.length} step{steps.length !== 1 ? 's' : ''} — click nodes to inspect
                  </span>
                )}
              </p>
              <TraceGraph run={detail} steps={steps} />
            </div>
          </div>
        </>
      )}
    </div>
  )
}
