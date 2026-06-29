'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { evalsAPI, agentsAPI } from '@/lib/api'
import type { EvalAnalysis, EvalAnalysisFix, EvalResult, EvalRun } from '@/types'
import {
  CheckCircle2, XCircle, AlertCircle, ExternalLink,
  Sparkles, Wrench, GraduationCap, FileText, Copy, Check, Loader2,
} from 'lucide-react'

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

function FixIcon({ type }: { type: EvalAnalysisFix['type'] }) {
  if (type === 'tool') return <Wrench className="w-4 h-4 text-purple-500 flex-shrink-0" />
  if (type === 'skill') return <GraduationCap className="w-4 h-4 text-blue-500 flex-shrink-0" />
  return <FileText className="w-4 h-4 text-indigo-500 flex-shrink-0" />
}

function PromptFix({ fix, agentId }: { fix: EvalAnalysisFix; agentId?: string }) {
  const [copied, setCopied] = useState(false)
  const [applying, setApplying] = useState(false)
  const [applied, setApplied] = useState(false)
  const [showConfirm, setShowConfirm] = useState(false)

  const handleCopy = () => {
    if (!fix.content) return
    navigator.clipboard.writeText(fix.content).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  const handleApply = async () => {
    if (!agentId || !fix.content) return
    setApplying(true)
    try {
      const agent = await agentsAPI.get(agentId) as { instructions?: string }
      const current = agent.instructions ?? ''
      await agentsAPI.update(agentId, { instructions: current + '\n\n' + fix.content })
      setApplied(true)
      setShowConfirm(false)
    } catch {
      // ignore
    } finally {
      setApplying(false)
    }
  }

  return (
    <div>
      <pre className="text-xs text-gray-800 bg-indigo-50 rounded p-3 whitespace-pre-wrap font-mono leading-relaxed">
        {fix.content}
      </pre>
      <div className="flex items-center gap-2 mt-2">
        <button
          onClick={handleCopy}
          className="flex items-center gap-1 text-xs text-gray-500 hover:text-gray-700 transition-colors"
        >
          {copied ? <Check className="w-3 h-3 text-green-500" /> : <Copy className="w-3 h-3" />}
          {copied ? 'Copied' : 'Copy'}
        </button>
        {agentId && !applied && (
          <>
            {showConfirm ? (
              <div className="flex items-center gap-2">
                <span className="text-xs text-gray-500">Append this to the agent&apos;s system prompt?</span>
                <button
                  onClick={handleApply}
                  disabled={applying}
                  className="text-xs bg-indigo-600 text-white px-2 py-0.5 rounded hover:bg-indigo-700 disabled:opacity-50 flex items-center gap-1"
                >
                  {applying && <Loader2 className="w-3 h-3 animate-spin" />}
                  Confirm
                </button>
                <button onClick={() => setShowConfirm(false)} className="text-xs text-gray-400 hover:text-gray-600">Cancel</button>
              </div>
            ) : (
              <button
                onClick={() => setShowConfirm(true)}
                className="text-xs text-indigo-600 hover:text-indigo-800 transition-colors"
              >
                Apply to agent
              </button>
            )}
          </>
        )}
        {applied && (
          <span className="text-xs text-green-600 flex items-center gap-1">
            <Check className="w-3 h-3" /> Applied
            {agentId && (
              <Link href={`/agents/${agentId}/edit`} className="ml-1 underline">
                View agent
              </Link>
            )}
          </span>
        )}
      </div>
    </div>
  )
}

function AnalysisCard({ analysis, agentId }: { analysis: EvalAnalysis; agentId?: string }) {
  return (
    <div className="border border-purple-200 rounded-lg overflow-hidden mb-6">
      <div className="bg-purple-50 px-4 py-3 flex items-center gap-2 border-b border-purple-200">
        <Sparkles className="w-4 h-4 text-purple-600" />
        <span className="text-sm font-semibold text-purple-800">AI Analysis</span>
      </div>
      <div className="p-4 sm:p-5 space-y-4">
        <div>
          <p className="text-xs font-medium text-gray-500 uppercase tracking-wide mb-1">What went wrong</p>
          <p className="text-sm text-gray-800">{analysis.issues}</p>
        </div>
        {analysis.fixes && analysis.fixes.length > 0 && (
          <div className="space-y-3">
            <p className="text-xs font-medium text-gray-500 uppercase tracking-wide">Suggested fixes</p>
            {analysis.fixes.map((fix, i) => (
              <div key={i} className="border border-gray-100 rounded-lg p-3 sm:p-4">
                <div className="flex items-start gap-2 mb-2">
                  <FixIcon type={fix.type} />
                  <div className="flex-1 min-w-0">
                    <span className="text-xs font-semibold uppercase tracking-wide text-gray-400 mr-2">
                      {fix.type === 'prompt' ? 'Prompt addition' : fix.type === 'tool' ? 'Missing tool' : 'Missing skill'}
                    </span>
                    <span className="text-sm text-gray-700">{fix.description}</span>
                  </div>
                </div>
                {fix.type === 'prompt' && fix.content && (
                  <PromptFix fix={fix} agentId={agentId} />
                )}
                {fix.type === 'tool' && (
                  <div className="mt-2">
                    <Link href="/tools" className="text-xs text-purple-600 hover:underline">Go to Tools →</Link>
                  </div>
                )}
                {fix.type === 'skill' && agentId && (
                  <div className="mt-2">
                    <Link href={`/agents/${agentId}/edit`} className="text-xs text-blue-600 hover:underline">Edit agent skills →</Link>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

export default function RunDetailPage({ params }: { params: { id: string; runId: string } }) {
  const { id, runId } = params
  const [run, setRun] = useState<EvalRun | null>(null)
  const [results, setResults] = useState<EvalResult[]>([])
  const [loading, setLoading] = useState(true)
  const [expanded, setExpanded] = useState<string | null>(null)
  const [analyzing, setAnalyzing] = useState(false)

  const load = useCallback(() =>
    evalsAPI.getRun(runId).then((r: unknown) => {
      const res = r as { run: EvalRun; results: EvalResult[] }
      setRun(res.run)
      setResults(res.results)
    }).catch(() => {}).finally(() => setLoading(false)),
  [runId])

  useEffect(() => { load() }, [load])

  // Poll while running or while analysis is pending (completed with failures but no analysis yet)
  useEffect(() => {
    if (!run) return
    const isRunning = run.status === 'pending' || run.status === 'running'
    const needsAnalysisPoll = run.status === 'completed' && run.failed > 0 && !run.analysis
    if (!isRunning && !needsAnalysisPoll) return
    const t = setInterval(load, 2000)
    return () => clearInterval(t)
  }, [run?.status, run?.failed, run?.analysis, load])

  const handleAnalyze = async () => {
    if (!run) return
    setAnalyzing(true)
    try {
      const res = await evalsAPI.analyzeRun(runId) as { analysis: EvalAnalysis | null }
      if (res.analysis) {
        setRun(prev => prev ? { ...prev, analysis: res.analysis ?? undefined } : prev)
      }
    } catch {
      // ignore
    } finally {
      setAnalyzing(false)
    }
  }

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

      {/* AI Analysis card — auto-shown when available */}
      {run.status === 'completed' && run.failed > 0 && (
        run.analysis ? (
          <AnalysisCard analysis={run.analysis} agentId={run.agent_id} />
        ) : (
          <div className="border border-dashed border-purple-200 rounded-lg p-4 mb-6 flex items-center justify-between flex-wrap gap-3">
            <div className="flex items-center gap-2 text-sm text-purple-700">
              <Sparkles className="w-4 h-4" />
              <span>
                {analyzing
                  ? 'Analyzing failures…'
                  : 'AI analysis is computing in the background or can be triggered manually.'}
              </span>
            </div>
            {!analyzing && (
              <button
                onClick={handleAnalyze}
                className="flex items-center gap-1.5 text-sm text-purple-700 border border-purple-300 rounded px-3 py-1.5 hover:bg-purple-50 transition-colors"
              >
                <Sparkles className="w-3.5 h-3.5" />
                Analyze failures
              </button>
            )}
            {analyzing && <Loader2 className="w-4 h-4 text-purple-500 animate-spin" />}
          </div>
        )
      )}

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
