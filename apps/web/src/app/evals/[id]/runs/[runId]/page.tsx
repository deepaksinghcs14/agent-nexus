'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { use } from 'react'
import { evalsAPI } from '@/lib/api'
import type { EvalResult, EvalRun } from '@/types'
import { CheckCircle2, XCircle, AlertCircle, ExternalLink } from 'lucide-react'

function StatusIcon({ passed, error }: { passed?: boolean; error?: string }) {
  if (error) return <AlertCircle className="w-4 h-4 text-amber-500 flex-shrink-0" />
  if (passed === true) return <CheckCircle2 className="w-4 h-4 text-green-500 flex-shrink-0" />
  if (passed === false) return <XCircle className="w-4 h-4 text-red-400 flex-shrink-0" />
  return <span className="w-4 h-4 flex-shrink-0" />
}

function RunStatusBadge({ status }: { status: EvalRun['status'] }) {
  const cls = {
    pending: 'bg-gray-100 text-gray-500',
    running: 'bg-blue-50 text-blue-600',
    completed: 'bg-green-50 text-green-700',
    failed: 'bg-red-50 text-red-600',
  }[status] ?? 'bg-gray-100 text-gray-500'
  return <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${cls}`}>{status}</span>
}

export default function RunDetailPage({ params }: { params: Promise<{ id: string; runId: string }> }) {
  const { id, runId } = use(params)
  const [run, setRun] = useState<EvalRun | null>(null)
  const [results, setResults] = useState<EvalResult[]>([])
  const [loading, setLoading] = useState(true)
  const [expanded, setExpanded] = useState<string | null>(null)

  const load = () =>
    evalsAPI.getRun(runId).then((r: unknown) => {
      const res = r as { run: EvalRun; results: EvalResult[] }
      setRun(res.run)
      setResults(res.results)
    }).catch(() => {}).finally(() => setLoading(false))

  useEffect(() => { load() }, [runId])

  // Poll while running
  useEffect(() => {
    if (!run || (run.status !== 'pending' && run.status !== 'running')) return
    const t = setInterval(load, 2000)
    return () => clearInterval(t)
  }, [run?.status])

  if (loading) return <div className="p-6 text-sm text-gray-400">Loading…</div>
  if (!run) return <div className="p-6 text-sm text-gray-500">Run not found.</div>

  const pct = run.total_cases > 0 ? Math.round((run.passed / (run.passed + run.failed || 1)) * 100) : null
  const dur = run.started_at && run.completed_at
    ? Math.round((new Date(run.completed_at).getTime() - new Date(run.started_at).getTime()) / 1000) + 's'
    : null

  return (
    <div className="p-4 sm:p-6">
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-gray-400 mb-4">
        <Link href="/evals" className="hover:text-gray-600 transition-colors">Evals</Link>
        <span>/</span>
        <Link href={`/evals/${id}`} className="hover:text-gray-600 transition-colors">Suite</Link>
        <span>/</span>
        <span className="text-gray-600">Run</span>
      </div>

      {/* Run summary */}
      <div className="bg-white border border-gray-200 rounded-lg p-4 sm:p-5 mb-6">
        <div className="flex flex-wrap items-center gap-4 justify-between">
          <div className="flex items-center gap-3">
            <RunStatusBadge status={run.status} />
            <span className="text-sm text-gray-500">{new Date(run.created_at).toLocaleString()}</span>
            {dur && <span className="text-sm text-gray-400">{dur}</span>}
          </div>
          {pct != null && run.status === 'completed' && (
            <div className="flex items-center gap-4 text-sm">
              <span className="text-green-600 font-medium">{run.passed} passed</span>
              <span className="text-red-500 font-medium">{run.failed} failed</span>
              {run.error_count > 0 && <span className="text-amber-500 font-medium">{run.error_count} errors</span>}
              <span className={`text-lg font-bold ${pct >= 80 ? 'text-green-600' : pct >= 50 ? 'text-amber-600' : 'text-red-600'}`}>{pct}%</span>
            </div>
          )}
          {(run.status === 'pending' || run.status === 'running') && (
            <div className="flex items-center gap-2 text-sm text-blue-600">
              <span className="inline-block w-2 h-2 bg-blue-500 rounded-full animate-pulse" />
              Running {results.length} / {run.total_cases} cases…
            </div>
          )}
        </div>

        {/* Progress bar */}
        {run.status === 'completed' && run.total_cases > 0 && (
          <div className="mt-4 h-2 rounded-full bg-gray-100 overflow-hidden">
            <div className="h-full bg-green-500 transition-all" style={{ width: `${pct}%` }} />
          </div>
        )}
      </div>

      {/* Results */}
      {results.length === 0 && run.status !== 'running' && run.status !== 'pending' ? (
        <p className="text-sm text-gray-400 py-6 text-center">No results yet.</p>
      ) : (
        <div className="space-y-3">
          {results.map((res, i) => (
            <div
              key={res.id}
              className={`border rounded-lg overflow-hidden transition-colors ${res.passed === true ? 'border-green-100' : res.error ? 'border-amber-100' : 'border-red-100'}`}
            >
              {/* Header row */}
              <button
                className="w-full flex items-start gap-3 p-4 text-left hover:bg-gray-50 transition-colors"
                onClick={() => setExpanded(expanded === res.id ? null : res.id)}
              >
                <StatusIcon passed={res.passed} error={res.error} />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-3 mb-1">
                    <span className="text-xs font-medium text-gray-400">Case {i + 1}</span>
                    {res.score > 0 && <span className="text-xs text-gray-500">Score: {res.score.toFixed(2)}</span>}
                    <span className="text-xs text-gray-400">{res.latency_ms}ms</span>
                    {res.run_id && (
                      <Link
                        href={`/runs?id=${res.run_id}`}
                        onClick={e => e.stopPropagation()}
                        className="text-xs text-purple-600 hover:underline flex items-center gap-0.5"
                      >
                        View run <ExternalLink className="w-3 h-3" />
                      </Link>
                    )}
                  </div>
                  <p className="text-sm text-gray-900 line-clamp-2">{res.input}</p>
                </div>
              </button>

              {/* Expanded detail */}
              {expanded === res.id && (
                <div className="px-4 pb-4 pt-0 border-t border-gray-100 space-y-3">
                  {res.actual_output && (
                    <div>
                      <p className="text-xs font-medium text-gray-500 mb-1">Actual output</p>
                      <p className="text-sm text-gray-800 whitespace-pre-wrap bg-gray-50 rounded p-3">{res.actual_output}</p>
                    </div>
                  )}
                  {res.expected_output && (
                    <div>
                      <p className="text-xs font-medium text-gray-500 mb-1">Expected output</p>
                      <p className="text-sm text-gray-700 whitespace-pre-wrap bg-green-50 rounded p-3">{res.expected_output}</p>
                    </div>
                  )}
                  {res.grading_criteria && (
                    <div>
                      <p className="text-xs font-medium text-gray-500 mb-1">Grading criteria</p>
                      <p className="text-sm text-gray-700 italic">{res.grading_criteria}</p>
                    </div>
                  )}
                  {res.judge_reasoning && (
                    <div>
                      <p className="text-xs font-medium text-gray-500 mb-1">Judge reasoning</p>
                      <p className="text-sm text-gray-700 bg-blue-50 rounded p-3">{res.judge_reasoning}</p>
                    </div>
                  )}
                  {res.error && (
                    <div>
                      <p className="text-xs font-medium text-amber-600 mb-1">Error</p>
                      <p className="text-sm text-amber-700 bg-amber-50 rounded p-3">{res.error}</p>
                    </div>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
