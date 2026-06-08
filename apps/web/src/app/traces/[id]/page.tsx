'use client'

import Link from 'next/link'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, CheckCircle, Clock, GitCommitHorizontal, XCircle } from 'lucide-react'
import { runsAPI } from '@/lib/api'
import { formatCost, formatTokens, statusColor } from '@/lib/utils'
import type { Run, RunStep } from '@/types'

export default function TraceDetailPage({ params }: { params: { id: string } }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['run', params.id],
    queryFn: () => runsAPI.get(params.id) as Promise<{ run: Run; steps: RunStep[] }>,
  })
  const run = data?.run
  const steps = data?.steps ?? []

  return <div className="p-6 max-w-6xl">
    <Link href="/traces" className="inline-flex items-center gap-1 text-[12px] text-gray-500 hover:text-gray-700 mb-5"><ArrowLeft className="w-3 h-3" /> Traces</Link>
    {error && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3">{(error as Error).message}</div>}
    {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading trace…</div>}
    {run && <><div className="flex items-start justify-between mb-5"><div><h1 className="font-mono text-sm font-medium text-gray-900">{run.id}</h1><p className="text-[11px] text-gray-400 mt-1">{new Date(run.started_at).toLocaleString()}</p></div><span className={`text-[11px] px-2 py-1 rounded-full ${statusColor(run.status)}`}>{run.status}</span></div>
      <div className="grid grid-cols-4 gap-3 mb-6">{[['Input tokens', formatTokens(run.total_input_tokens)], ['Output tokens', formatTokens(run.total_output_tokens)], ['Estimated cost', formatCost(run.cost_estimate)], ['Steps', String(steps.length)]].map(([label, value]) => <div key={label} className="bg-gray-50 rounded-lg p-3"><p className="text-[10px] text-gray-400">{label}</p><p className="text-lg font-medium text-gray-900 mt-1">{value}</p></div>)}</div>
      <div className="grid grid-cols-2 gap-5"><div><p className="text-[12px] font-medium text-gray-700 mb-2">Execution timeline</p><div className="bg-white border border-gray-100 rounded-xl overflow-hidden">{steps.map((step, index) => <div key={step.id} className="flex gap-3 p-4 border-b last:border-b-0 border-gray-50"><div className="flex flex-col items-center">{step.error ? <XCircle className="w-4 h-4 text-red-500" /> : <CheckCircle className="w-4 h-4 text-green-500" />}{index < steps.length - 1 && <div className="w-px flex-1 bg-gray-100 mt-2" />}</div><div className="flex-1 min-w-0"><div className="flex items-center"><p className="text-[12px] font-medium text-gray-800 capitalize">{step.step_type.replaceAll('_', ' ')}</p><span className="ml-auto inline-flex items-center gap-1 text-[10px] text-gray-400"><Clock className="w-3 h-3" /> {step.latency_ms}ms</span></div>{step.tool_name && <p className="text-[11px] text-gray-500 mt-1">Tool: {step.tool_name}</p>}{step.error && <p className="text-[11px] text-red-600 mt-1">{step.error}</p>}<details className="mt-2"><summary className="text-[10px] text-purple-600 cursor-pointer">Step data</summary><pre className="mt-2 p-2 bg-gray-50 rounded text-[10px] text-gray-600 overflow-auto">{JSON.stringify({ input: step.input, output: step.output }, null, 2)}</pre></details></div></div>)}{steps.length === 0 && <div className="p-10 text-center"><GitCommitHorizontal className="mx-auto text-gray-300 mb-2" /><p className="text-[12px] text-gray-400">This run has no recorded execution steps.</p></div>}</div></div>
      <div><p className="text-[12px] font-medium text-gray-700 mb-2">Input</p><div className="bg-gray-50 border border-gray-100 rounded-lg p-3 text-[12px] text-gray-600 whitespace-pre-wrap">{run.input || 'No input recorded.'}</div><p className="text-[12px] font-medium text-gray-700 mt-4 mb-2">Output</p><div className="bg-gray-50 border border-gray-100 rounded-lg p-3 text-[12px] text-gray-600 whitespace-pre-wrap">{run.output || run.error_message || 'No output recorded.'}</div></div></div>
    </>}
  </div>
}
