'use client'

import Link from 'next/link'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Ban, Check, Clock, GitCommitHorizontal } from 'lucide-react'
import { runsAPI } from '@/lib/api'
import { formatCost, formatTokens, statusColor } from '@/lib/utils'
import type { Run, RunStep } from '@/types'

type RunDetail = Run & { steps?: RunStep[] }

export default function RunDetailPage({ params }: { params: { id: string } }) {
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useQuery({ queryKey: ['run', params.id], queryFn: () => runsAPI.get(params.id) as Promise<RunDetail | { run: Run; steps: RunStep[] }> })
  const cancel = useMutation({ mutationFn: () => runsAPI.cancel(params.id), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['run', params.id] }) })
  const detail: RunDetail | undefined = data
    ? 'run' in data
      ? { ...data.run, steps: data.steps }
      : data as RunDetail
    : undefined
  const steps = detail?.steps ?? []

  return <div className="p-6 max-w-5xl">
    <div className="flex items-center justify-between mb-5"><Link href="/runs" className="text-[12px] text-gray-500 hover:text-gray-700 inline-flex items-center gap-1"><ArrowLeft className="w-3 h-3" /> Runs</Link>{detail && ['pending', 'running', 'approval_wait'].includes(detail.status) && <button onClick={() => cancel.mutate()} className="inline-flex items-center gap-1.5 px-3 py-1.5 border border-red-200 text-red-600 text-[12px] rounded-lg"><Ban className="w-3.5 h-3.5" /> Cancel run</button>}</div>
    {error && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3">{(error as Error).message}</div>}
    {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading trace…</div>}
    {detail && <>
      <div className="grid grid-cols-6 gap-3 mb-6">{[
        ['Status', detail.status], ['Input tokens', formatTokens(detail.total_input_tokens)], ['Output tokens', formatTokens(detail.total_output_tokens)], ['Cost', formatCost(detail.cost_estimate)], ['Started', new Date(detail.started_at).toLocaleString()], ['Completed', detail.completed_at ? new Date(detail.completed_at).toLocaleString() : '—'],
      ].map(([label, value]) => <div key={label} className="bg-gray-50 rounded-lg p-2.5"><p className="text-[10px] text-gray-400 mb-1">{label}</p>{label === 'Status' ? <span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(value)}`}>{value}</span> : <p className="text-[11px] font-medium text-gray-800 break-words">{value}</p>}</div>)}</div>
      <div className="grid grid-cols-2 gap-5">
        <div><p className="text-[12px] font-medium text-gray-700 mb-2">Input</p><div className="bg-gray-50 border border-gray-100 rounded-lg p-3 text-[12px] text-gray-600 whitespace-pre-wrap">{detail.input || 'No input recorded.'}</div><p className="text-[12px] font-medium text-gray-700 mt-4 mb-2">Output</p><div className="bg-gray-50 border border-gray-100 rounded-lg p-3 text-[12px] text-gray-600 whitespace-pre-wrap">{detail.output || detail.error_message || 'No output recorded.'}</div></div>
        <div><p className="text-[12px] font-medium text-gray-700 mb-2">Run steps</p><div className="bg-white border border-gray-100 rounded-xl overflow-hidden">{steps.map((step) => <div key={step.id} className="p-3 border-b last:border-b-0 border-gray-50"><div className="flex items-center gap-2"><Check className="w-3.5 h-3.5 text-green-500" /><span className="text-[12px] font-medium text-gray-800">{step.step_type.replaceAll('_', ' ')}</span><span className="ml-auto inline-flex items-center gap-1 text-[10px] text-gray-400"><Clock className="w-3 h-3" /> {step.latency_ms}ms</span></div>{step.tool_name && <p className="text-[11px] text-gray-500 mt-1">Tool: {step.tool_name}</p>}{step.error && <p className="text-[11px] text-red-600 mt-1">{step.error}</p>}</div>)}{steps.length === 0 && <div className="p-8 text-center"><GitCommitHorizontal className="mx-auto text-gray-300 mb-2" /><p className="text-[12px] text-gray-400">No steps recorded for this run.</p></div>}</div></div>
      </div>
    </>}
  </div>
}
