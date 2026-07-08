'use client'

import { use, useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { evalsAPI, agentsAPI } from '@/lib/api'
import type { EvalAnalysis, EvalAnalysisFix, EvalResult, EvalRun } from '@/types'
import {
  CheckCircle2, XCircle, AlertCircle, ExternalLink,
  Sparkles, Wrench, GraduationCap, FileText, Copy, Check, Loader2, ThumbsUp, ThumbsDown,
} from 'lucide-react'

// Effective pass/fail — override takes precedence over auto-grade
function effectivePassed(res: EvalResult): boolean | undefined {
  if (res.override_passed !== null && res.override_passed !== undefined) return res.override_passed
  return res.passed
}

function StatusIcon({ res }: { res: EvalResult }) {
  const ep = effectivePassed(res)
  const isOverridden = res.override_passed !== null && res.override_passed !== undefined
  if (res.error && ep === undefined) return <AlertCircle className="w-4 h-4 text-warn flex-shrink-0" />
  if (ep === true) return <CheckCircle2 className={`w-4 h-4 flex-shrink-0 ${isOverridden ? 'text-good' : 'text-good'}`} />
  if (ep === false) return <XCircle className={`w-4 h-4 flex-shrink-0 ${isOverridden ? 'text-crit' : 'text-crit'}`} />
  return <span className="w-4 h-4 flex-shrink-0" />
}

function RunStatusBadge({ status }: { status: EvalRun['status'] }) {
  const cls = {
    pending: 'bg-muted text-muted-foreground',
    running: 'bg-info/10 text-info',
    completed: 'bg-good/10 text-good',
    failed: 'bg-crit/10 text-crit',
  }[status] ?? 'bg-muted text-muted-foreground'
  return <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${cls}`}>{status}</span>
}

function FixIcon({ type }: { type: EvalAnalysisFix['type'] }) {
  if (type === 'tool') return <Wrench className="w-4 h-4 text-accent dark:text-accent-bright flex-shrink-0" />
  if (type === 'skill') return <GraduationCap className="w-4 h-4 text-info flex-shrink-0" />
  return <FileText className="w-4 h-4 text-accent dark:text-accent-bright flex-shrink-0" />
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
      <pre className="text-xs text-foreground bg-accent/10 dark:bg-info/10 rounded p-3 whitespace-pre-wrap font-mono leading-relaxed">
        {fix.content}
      </pre>
      <div className="flex items-center gap-2 mt-2">
        <button
          onClick={handleCopy}
          className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground dark:hover:text-faint transition-colors"
        >
          {copied ? <Check className="w-3 h-3 text-good" /> : <Copy className="w-3 h-3" />}
          {copied ? 'Copied' : 'Copy'}
        </button>
        {agentId && !applied && (
          <>
            {showConfirm ? (
              <div className="flex items-center gap-2">
                <span className="text-xs text-muted-foreground">Append this to the agent&apos;s system prompt?</span>
                <button
                  onClick={handleApply}
                  disabled={applying}
                  className="text-xs bg-accent text-white px-2 py-0.5 rounded hover:bg-accent-hover disabled:opacity-50 flex items-center gap-1"
                >
                  {applying && <Loader2 className="w-3 h-3 animate-spin" />}
                  Confirm
                </button>
                <button onClick={() => setShowConfirm(false)} className="text-xs text-faint hover:text-muted-foreground dark:hover:text-faint">Cancel</button>
              </div>
            ) : (
              <button
                onClick={() => setShowConfirm(true)}
                className="text-xs text-info dark:text-info hover:text-accent dark:text-accent-bright transition-colors"
              >
                Apply to agent
              </button>
            )}
          </>
        )}
        {applied && (
          <span className="text-xs text-good dark:text-good flex items-center gap-1">
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
    <div className="border border-accent/40 rounded-lg overflow-hidden mb-6">
      <div className="bg-accent/10 px-4 py-3 flex items-center gap-2 border-b border-accent/40">
        <Sparkles className="w-4 h-4 text-accent dark:text-accent-bright" />
        <span className="text-sm font-semibold text-accent dark:text-accent-bright dark:text-accent dark:text-accent-bright">AI Analysis</span>
      </div>
      <div className="p-4 sm:p-5 space-y-4">
        <div>
          <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide mb-1">What went wrong</p>
          <p className="text-sm text-foreground">{analysis.issues}</p>
        </div>
        {analysis.fixes && analysis.fixes.length > 0 && (
          <div className="space-y-3">
            <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">Suggested fixes</p>
            {analysis.fixes.map((fix, i) => (
              <div key={i} className="border border-border rounded-lg p-3 sm:p-4">
                <div className="flex items-start gap-2 mb-2">
                  <FixIcon type={fix.type} />
                  <div className="flex-1 min-w-0">
                    <span className="text-xs font-semibold uppercase tracking-wide text-faint mr-2">
                      {fix.type === 'prompt' ? 'Prompt addition' : fix.type === 'tool' ? 'Missing tool' : 'Missing skill'}
                    </span>
                    <span className="text-sm text-foreground">{fix.description}</span>
                  </div>
                </div>
                {fix.type === 'prompt' && fix.content && (
                  <PromptFix fix={fix} agentId={agentId} />
                )}
                {fix.type === 'tool' && (
                  <div className="mt-2">
                    <Link href="/tools" className="text-xs text-accent dark:text-accent-bright hover:underline">Go to Tools →</Link>
                  </div>
                )}
                {fix.type === 'skill' && agentId && (
                  <div className="mt-2">
                    <Link href={`/agents/${agentId}/edit`} className="text-xs text-info hover:underline">Edit agent skills →</Link>
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

function ResultCard({
  res, index, runId, expanded, onToggle, onResultUpdate,
}: {
  res: EvalResult
  index: number
  runId: string
  expanded: boolean
  onToggle: () => void
  onResultUpdate: (r: EvalResult) => void
}) {
  const [overriding, setOverriding] = useState(false)
  const [fixing, setFixing] = useState(false)
  const [fixResult, setFixResult] = useState<{ expected_output: string; grading_criteria: string } | null>(null)

  const ep = effectivePassed(res)
  const isOverridden = res.override_passed !== null && res.override_passed !== undefined
  const borderCls = ep === true ? 'border-good/30' : res.error && ep === undefined ? 'border-warn/30' : 'border-crit/30'

  const handleOverride = async (val: boolean | null) => {
    setOverriding(true)
    try {
      await evalsAPI.overrideResult(runId, res.id, val)
      onResultUpdate({ ...res, override_passed: val })
    } catch { /* ignore */ } finally {
      setOverriding(false)
    }
  }

  const handleFixCase = async () => {
    setFixing(true)
    setFixResult(null)
    try {
      const r = await evalsAPI.fixCase(runId, res.id) as { expected_output: string; grading_criteria: string }
      setFixResult(r)
      // Optimistically update displayed case fields
      onResultUpdate({ ...res, expected_output: r.expected_output, grading_criteria: r.grading_criteria })
    } catch { /* ignore */ } finally {
      setFixing(false)
    }
  }

  return (
    <div className={`border rounded-lg overflow-hidden transition-colors ${borderCls}`}>
      {/* Header row */}
      <button
        className="w-full flex items-start gap-3 p-4 text-left hover:bg-muted transition-colors"
        onClick={onToggle}
      >
        <StatusIcon res={res} />
        <div className="flex-1 min-w-0">
          <div className="flex flex-wrap items-center gap-2 mb-1">
            <span className="text-xs font-medium text-faint">Case {index + 1}</span>
            {res.score > 0 && <span className="text-xs text-muted-foreground">Score: {res.score.toFixed(2)}</span>}
            <span className="text-xs text-faint">{res.latency_ms}ms</span>
            {isOverridden && (
              <span className="text-xs px-1.5 py-0.5 rounded bg-warn/10 dark:bg-warn/100/10 text-warn dark:text-warn border border-warn/30">
                manually {res.override_passed ? 'correct' : 'incorrect'}
              </span>
            )}
            {res.run_id && (
              <Link
                href={`/runs?id=${res.run_id}`}
                onClick={e => e.stopPropagation()}
                className="text-xs text-accent dark:text-accent-bright hover:underline flex items-center gap-0.5"
              >
                View run <ExternalLink className="w-3 h-3" />
              </Link>
            )}
          </div>
          <p className="text-sm text-foreground line-clamp-2">{res.input}</p>
        </div>
        {/* Override buttons — always visible, stop propagation so they don't toggle expand */}
        <div className="flex items-center gap-1 flex-shrink-0 ml-2" onClick={e => e.stopPropagation()}>
          {overriding ? (
            <Loader2 className="w-3.5 h-3.5 text-faint animate-spin" />
          ) : (
            <>
              <button
                title="Mark as correct"
                onClick={() => handleOverride(res.override_passed === true ? null : true)}
                className={`p-1 rounded transition-colors ${res.override_passed === true ? 'bg-good/15 text-good dark:text-good' : 'text-faint hover:text-good hover:bg-good/10'}`}
              >
                <ThumbsUp className="w-3.5 h-3.5" />
              </button>
              <button
                title="Mark as incorrect"
                onClick={() => handleOverride(res.override_passed === false ? null : false)}
                className={`p-1 rounded transition-colors ${res.override_passed === false ? 'bg-crit/15 text-crit' : 'text-faint hover:text-crit hover:bg-crit/10'}`}
              >
                <ThumbsDown className="w-3.5 h-3.5" />
              </button>
            </>
          )}
        </div>
      </button>

      {/* Expanded detail */}
      {expanded && (
        <div className="px-4 pb-4 pt-0 border-t border-border space-y-3">
          {res.actual_output && (
            <div>
              <p className="text-xs font-medium text-muted-foreground mb-1">Actual output</p>
              <p className="text-sm text-foreground whitespace-pre-wrap bg-muted rounded p-3">{res.actual_output}</p>
            </div>
          )}
          {res.expected_output && (
            <div>
              <p className="text-xs font-medium text-muted-foreground mb-1">Expected output</p>
              <p className="text-sm text-foreground whitespace-pre-wrap bg-good/10 rounded p-3">{res.expected_output}</p>
            </div>
          )}
          {res.grading_criteria && (
            <div>
              <p className="text-xs font-medium text-muted-foreground mb-1">Grading criteria</p>
              <p className="text-sm text-foreground italic">{res.grading_criteria}</p>
            </div>
          )}
          {res.judge_reasoning && (
            <div>
              <p className="text-xs font-medium text-muted-foreground mb-1">Judge reasoning</p>
              <p className="text-sm text-foreground bg-info/10 rounded p-3">{res.judge_reasoning}</p>
            </div>
          )}
          {res.error && (
            <div>
              <p className="text-xs font-medium text-warn dark:text-warn mb-1">Error</p>
              <p className="text-sm text-warn bg-warn/10 rounded p-3">{res.error}</p>
            </div>
          )}

          {/* AI fix case — shown when manually overridden */}
          {isOverridden && (
            <div className="pt-1 border-t border-border">
              {fixResult ? (
                <div className="flex items-center gap-2 text-xs text-good dark:text-good">
                  <Check className="w-3.5 h-3.5" />
                  Case updated — expected output and grading criteria refined.
                </div>
              ) : (
                <button
                  onClick={handleFixCase}
                  disabled={fixing}
                  className="flex items-center gap-1.5 text-xs text-accent dark:text-accent-bright border border-accent/40 rounded px-2.5 py-1.5 hover:bg-accent/10 transition-colors disabled:opacity-50"
                >
                  {fixing ? <Loader2 className="w-3 h-3 animate-spin" /> : <Sparkles className="w-3 h-3" />}
                  {fixing ? 'AI is refining the case…' : 'Fix case with AI'}
                </button>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export default function RunDetailPage({ params }: { params: Promise<{ id: string; runId: string }> }) {
  const { id, runId } = use(params)
  const [run, setRun] = useState<EvalRun | null>(null)
  const [results, setResults] = useState<EvalResult[]>([])
  const [loading, setLoading] = useState(true)
  const [expanded, setExpanded] = useState<string | null>(null)
  const [analyzing, setAnalyzing] = useState(false)
  const [autoAnalyzeTriggered, setAutoAnalyzeTriggered] = useState(false)

  const load = useCallback(() =>
    evalsAPI.getRun(runId).then((r: unknown) => {
      const res = r as { run: EvalRun; results: EvalResult[] }
      setRun(res.run)
      setResults(res.results)
    }).catch(() => {}).finally(() => setLoading(false)),
  [runId])

  useEffect(() => { load() }, [load])

  // Auto-trigger analysis on page load for completed runs with failures and no analysis yet
  useEffect(() => {
    if (!run || autoAnalyzeTriggered) return
    if (run.status === 'completed' && run.failed > 0 && !run.analysis) {
      setAutoAnalyzeTriggered(true)
      setAnalyzing(true)
      evalsAPI.analyzeRun(runId).then((res: unknown) => {
        const r = res as { analysis: EvalAnalysis | null }
        if (r.analysis) {
          setRun(prev => prev ? { ...prev, analysis: r.analysis ?? undefined } : prev)
        }
      }).catch(() => {}).finally(() => setAnalyzing(false))
    }
  }, [run?.status, run?.failed, run?.analysis, autoAnalyzeTriggered, runId])

  // Poll while run is still executing
  useEffect(() => {
    if (!run) return
    if (run.status !== 'pending' && run.status !== 'running') return
    const t = setInterval(load, 2000)
    return () => clearInterval(t)
  }, [run?.status, load])

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

  if (loading) return <div className="p-6 text-sm text-faint">Loading…</div>
  if (!run) return <div className="p-6 text-sm text-muted-foreground">Run not found.</div>

  const pct = run.total_cases > 0 ? Math.round((run.passed / (run.passed + run.failed || 1)) * 100) : null
  const dur = run.started_at && run.completed_at
    ? Math.round((new Date(run.completed_at).getTime() - new Date(run.started_at).getTime()) / 1000) + 's'
    : null

  return (
    <div className="p-4 sm:p-6">
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-faint mb-4">
        <Link href="/evals" className="hover:text-muted-foreground dark:hover:text-faint transition-colors">Evals</Link>
        <span>/</span>
        <Link href={`/evals/${id}`} className="hover:text-muted-foreground dark:hover:text-faint transition-colors">Suite</Link>
        <span>/</span>
        <span className="text-muted-foreground">Run</span>
      </div>

      {/* Run summary */}
      <div className="bg-surface border border-border-strong rounded-lg p-4 sm:p-5 mb-6">
        <div className="flex flex-wrap items-center gap-4 justify-between">
          <div className="flex items-center gap-3">
            <RunStatusBadge status={run.status} />
            <span className="text-sm text-muted-foreground">{new Date(run.created_at).toLocaleString()}</span>
            {dur && <span className="text-sm text-faint">{dur}</span>}
          </div>
          {pct != null && run.status === 'completed' && (
            <div className="flex items-center gap-4 text-sm">
              <span className="text-good dark:text-good font-medium">{run.passed} passed</span>
              <span className="text-crit font-medium">{run.failed} failed</span>
              {run.error_count > 0 && <span className="text-warn font-medium">{run.error_count} errors</span>}
              <span className={`text-lg font-bold ${pct >= 80 ? 'text-good dark:text-good' : pct >= 50 ? 'text-warn dark:text-warn' : 'text-crit'}`}>{pct}%</span>
            </div>
          )}
          {(run.status === 'pending' || run.status === 'running') && (
            <div className="flex items-center gap-2 text-sm text-info">
              <span className="inline-block w-2 h-2 bg-info rounded-full animate-pulse" />
              Running {results.length} / {run.total_cases} cases…
            </div>
          )}
        </div>

        {/* Progress bar */}
        {run.status === 'completed' && run.total_cases > 0 && (
          <div className="mt-4 h-2 rounded-full bg-muted overflow-hidden">
            <div className="h-full bg-good transition-all" style={{ width: `${pct}%` }} />
          </div>
        )}
      </div>

      {/* AI Analysis card — auto-shown when available */}
      {run.status === 'completed' && run.failed > 0 && (
        run.analysis ? (
          <AnalysisCard analysis={run.analysis} agentId={run.agent_id} />
        ) : (
          <div className="border border-dashed border-accent/40 rounded-lg p-4 mb-6 flex items-center justify-between flex-wrap gap-3">
            <div className="flex items-center gap-2 text-sm text-accent dark:text-accent-bright">
              {analyzing
                ? <Loader2 className="w-4 h-4 animate-spin" />
                : <Sparkles className="w-4 h-4" />}
              <span>
                {analyzing ? 'Analyzing failures with AI…' : 'Analysis could not be generated. Retry below.'}
              </span>
            </div>
            {!analyzing && (
              <button
                onClick={handleAnalyze}
                className="flex items-center gap-1.5 text-sm text-accent dark:text-accent-bright border border-accent/50 rounded px-3 py-1.5 hover:bg-accent/10 transition-colors"
              >
                <Sparkles className="w-3.5 h-3.5" />
                Retry analysis
              </button>
            )}
          </div>
        )
      )}

      {/* Results */}
      {results.length === 0 && run.status !== 'running' && run.status !== 'pending' ? (
        <p className="text-sm text-faint py-6 text-center">No results yet.</p>
      ) : (
        <div className="space-y-3">
          {results.map((res, i) => (
            <ResultCard
              key={res.id}
              res={res}
              index={i}
              runId={runId}
              expanded={expanded === res.id}
              onToggle={() => setExpanded(expanded === res.id ? null : res.id)}
              onResultUpdate={(updated) => setResults(prev => prev.map(r => r.id === updated.id ? updated : r))}
            />
          ))}
        </div>
      )}
    </div>
  )
}
