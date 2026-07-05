'use client'

import { use, useState } from 'react'
import Link from 'next/link'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Ban, ChevronDown, ChevronRight, Loader2, Sparkles } from 'lucide-react'
import { agentsAPI, runsAPI } from '@/lib/api'
import { formatCost, formatTokens, statusColor } from '@/lib/utils'
import type { Agent, Run, RunStep } from '@/types'
import { TraceGraph, type RunWithWorkflow, STEP_STYLE } from './TraceGraph'

type ApprovalRequest = { id: string; tool_name: string; tool_input: unknown; status: string; created_at: string }
type RunDetail = Run & { steps?: RunStep[]; workflow_id?: string; approval_request?: ApprovalRequest }

const ROW_GRID = 'grid items-center gap-x-3 px-4' as const
const GRID_COLS = 'grid-cols-[20px_1fr_80px_48px_64px_56px]' as const
const ACTIVE_RUN_STATUSES = new Set(['pending', 'running', 'approval_wait', 'session_wait'])

function SubRunRow({ subRun, agentName }: { subRun: Run; agentName?: string }) {
  const [open, setOpen] = useState(false)
  const { data, isFetching } = useQuery({
    queryKey: ['run', subRun.id],
    queryFn: () => runsAPI.get(subRun.id) as Promise<{ run: Run; steps: RunStep[] }>,
    enabled: open,
    refetchInterval: open && ACTIVE_RUN_STATUSES.has(subRun.status) ? 2000 : false,
  })
  const steps = data?.steps ?? []
  const currentRun = data?.run ?? subRun
  const isActive = ACTIVE_RUN_STATUSES.has(currentRun.status)
  const output = (currentRun.output || currentRun.error_message || '').trim()
  const dur = subRun.completed_at
    ? Math.round((new Date(subRun.completed_at).getTime() - new Date(subRun.started_at).getTime()) / 100) / 10
    : null
  const tokens = subRun.total_input_tokens + subRun.total_output_tokens

  return (
    <div className="border-b border-gray-50 last:border-b-0">
      <button
        className={`w-full ${ROW_GRID} ${GRID_COLS} py-2.5 text-left hover:bg-gray-50/60 transition-colors`}
        onClick={() => setOpen(v => !v)}
      >
        <span className="flex items-center justify-center text-gray-300 dark:text-gray-600">
          {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        </span>
        <span className="flex items-center gap-1.5 min-w-0">
          <Sparkles size={11} className="text-indigo-400 flex-shrink-0" />
          <span className="text-[12px] font-medium text-gray-800 dark:text-gray-200 truncate">{agentName ?? 'Agent'}</span>
        </span>
        <span className="flex justify-center">
          <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${statusColor(subRun.status)}`}>{subRun.status}</span>
        </span>
        <span className="text-[11px] text-gray-400 dark:text-gray-500 text-right">{dur !== null ? `${dur}s` : '—'}</span>
        <span className="text-[11px] text-gray-400 dark:text-gray-500 text-right">{tokens > 0 ? formatTokens(tokens) : '—'}</span>
        <span className="text-[11px] text-gray-400 dark:text-gray-500 text-right">{subRun.cost_estimate > 0 ? formatCost(subRun.cost_estimate) : '—'}</span>
      </button>
      {open && (
        <div className="mx-4 mb-2 rounded-lg border border-gray-100 dark:border-gray-800 bg-gray-50/50 overflow-hidden">
          {isFetching && <p className="text-[11px] text-gray-400 dark:text-gray-500 text-center py-3">Loading steps…</p>}
          {isActive && (
            <div className="flex items-center gap-2 border-b border-gray-100 dark:border-gray-800 bg-white dark:bg-gray-900 px-3 py-2 text-[11px] text-gray-500 dark:text-gray-400">
              <Loader2 className="h-3 w-3 animate-spin" />
              Waiting for this run to finish…
            </div>
          )}
          {!isFetching && steps.length === 0 && !isActive && <p className="text-[11px] text-gray-400 dark:text-gray-500 text-center py-3">No steps recorded.</p>}
          {!isFetching && output && (
            <div className="border-b border-gray-100 dark:border-gray-800 bg-white dark:bg-gray-900 p-3">
              <p className="text-[10px] font-medium uppercase text-gray-400 dark:text-gray-500 mb-1.5">Output</p>
              <div className="text-[12px] text-gray-700 dark:text-gray-300 whitespace-pre-wrap max-h-52 overflow-y-auto">{output}</div>
            </div>
          )}
          {steps.map((step) => {
            const st = STEP_STYLE[step.step_type] ?? STEP_STYLE.model_call
            return (
              <div key={step.id} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 12px', borderBottom: '1px solid #f3f4f6' }}>
                <span style={{ color: st.border, flexShrink: 0 }}>{st.icon}</span>
                <span style={{ fontSize: 11, color: '#374151', flex: 1 }}>
                  {step.tool_name ? `${st.label}: ${step.tool_name}` : st.label}
                </span>
                {step.latency_ms > 0 && <span style={{ fontSize: 9, color: '#9ca3af' }}>{step.latency_ms}ms</span>}
                {step.tokens_used > 0 && <span style={{ fontSize: 9, color: '#9ca3af' }}>{step.tokens_used} tok</span>}
                {step.error && <span style={{ fontSize: 9, color: '#ef4444', fontWeight: 600 }}>ERR</span>}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

export default function RunDetailPage({ params }: { params: Promise<{ runId: string }> }) {
  const { runId } = use(params)
  const queryClient = useQueryClient()

  const { data, isLoading, error } = useQuery({
    queryKey: ['run', runId],
    queryFn: () => runsAPI.get(runId) as Promise<RunDetail | { run: Run; steps: RunStep[]; workflow_id?: string }>,
    refetchInterval: (query) => {
      const current = query.state.data as RunDetail | { run: Run; steps: RunStep[]; workflow_id?: string } | undefined
      const run = current ? ('run' in current ? current.run : current) : undefined
      return run && ACTIVE_RUN_STATUSES.has(run.status) ? 2000 : false
    },
  })

  const { data: childrenData } = useQuery({
    queryKey: ['run-children', runId],
    queryFn: () => runsAPI.listChildren(runId) as Promise<{ data: Run[] }>,
    refetchInterval: (query) => {
      const current = query.state.data as { data: Run[] } | undefined
      return current?.data.some((run) => ACTIVE_RUN_STATUSES.has(run.status)) ? 2000 : false
    },
  })

  const { data: agentData } = useQuery({
    queryKey: ['agents'],
    queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }>,
  })

  const cancel = useMutation({
    mutationFn: () => runsAPI.cancel(runId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['run', runId] }),
  })

  const decide = useMutation({
    mutationFn: (decision: 'approved' | 'rejected') => runsAPI.approve(runId, { decision }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['run', runId] }),
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
  const approvalRequest = data && 'approval_request' in (data as object)
    ? (data as { approval_request?: ApprovalRequest }).approval_request
    : undefined
  const steps = detail?.steps ?? []
  const isWorkflowRoot = !!detail && !detail.agent_id && !detail.parent_run_id
  const children = childrenData?.data ?? []
  const hasChildren = children.length > 0
  const hasActiveChildren = children.some((run) => ACTIVE_RUN_STATUSES.has(run.status))
  const isActive = !!detail && ACTIVE_RUN_STATUSES.has(detail.status)
  const agentNames = Object.fromEntries((agentData?.data ?? []).map((a: Agent) => [a.id, a.name]))

  return (
    <div className="p-4 sm:p-6 max-w-6xl">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-5">
        <Link href="/runs" className="text-[12px] text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300 inline-flex items-center gap-1">
          <ArrowLeft className="w-3 h-3" /> Runs
        </Link>
        {detail && ['pending', 'running', 'approval_wait', 'session_wait'].includes(detail.status) && (
          <button
            onClick={() => cancel.mutate()}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 border border-red-200 text-red-600 dark:text-red-300 text-[12px] rounded-lg"
          >
            <Ban className="w-3.5 h-3.5" /> Cancel run
          </button>
        )}
      </div>

      {error && (
        <div className="text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg p-3">{(error as Error).message}</div>
      )}
      {isLoading && (
        <div className="py-12 text-center text-sm text-gray-400 dark:text-gray-500">Loading trace…</div>
      )}

      {detail && (
        <>
          {/* Pending approval — shown for waiting runs, including runs parked
              across a server restart (no live playground session exists). */}
          {detail.status === 'approval_wait' && approvalRequest && (
            <div className="mb-5 rounded-xl border border-purple-200 bg-purple-50/60 p-4">
              <p className="text-[12px] font-semibold text-purple-900 mb-1">
                Approval required: <span className="font-mono">{approvalRequest.tool_name}</span>
              </p>
              <pre className="text-[11px] text-purple-900/80 bg-white/70 border border-purple-100 rounded-lg p-2.5 mb-3 max-h-40 overflow-auto whitespace-pre-wrap">
                {JSON.stringify(approvalRequest.tool_input, null, 2)}
              </pre>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => decide.mutate('approved')}
                  disabled={decide.isPending}
                  className="px-3 py-1.5 rounded-lg bg-purple-700 text-white text-[12px] font-medium disabled:opacity-50"
                >
                  {decide.isPending ? 'Sending…' : 'Approve & resume'}
                </button>
                <button
                  onClick={() => decide.mutate('rejected')}
                  disabled={decide.isPending}
                  className="px-3 py-1.5 rounded-lg border border-purple-300 text-purple-800 dark:text-purple-300 text-[12px] font-medium disabled:opacity-50"
                >
                  Reject
                </button>
                {decide.isError && (
                  <span className="text-[11px] text-red-600 dark:text-red-300">{(decide.error as Error).message}</span>
                )}
              </div>
            </div>
          )}
          {detail.status === 'session_wait' && (
            <div className="mb-5 rounded-xl border border-indigo-200 bg-indigo-50/60 p-4 text-[12px] text-indigo-900">
              A repo coding session is running for this run. It resumes automatically when the session completes — this can take minutes to hours.
            </div>
          )}

          {/* Metrics */}
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3 mb-5">
            {[
              ['Status', detail.status],
              ['Input tokens', formatTokens(detail.total_input_tokens)],
              ['Output tokens', formatTokens(detail.total_output_tokens)],
              ['Cost', formatCost(detail.cost_estimate)],
              ['Started', new Date(detail.started_at).toLocaleString()],
              ['Completed', detail.completed_at ? new Date(detail.completed_at).toLocaleString() : '—'],
            ].map(([label, value]) => (
              <div key={label} className="bg-gray-50 dark:bg-gray-800/60 rounded-lg p-2.5">
                <p className="text-[10px] text-gray-400 dark:text-gray-500 mb-1">{label}</p>
                {label === 'Status' ? (
                  <span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(value)}`}>{value}</span>
                ) : (
                  <p className="text-[11px] font-medium text-gray-800 dark:text-gray-200 break-words">{value}</p>
                )}
              </div>
            ))}
          </div>

          {/* Input / Output */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-5">
            <div>
              <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300 mb-1.5">Input</p>
              <div className="bg-gray-50 dark:bg-gray-800/60 border border-gray-100 dark:border-gray-800 rounded-lg p-3 text-[12px] text-gray-600 dark:text-gray-400 whitespace-pre-wrap max-h-32 overflow-y-auto">
                {detail.input || 'No input recorded.'}
              </div>
            </div>
            <div>
              <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300 mb-1.5">Output</p>
              <div className="bg-gray-50 dark:bg-gray-800/60 border border-gray-100 dark:border-gray-800 rounded-lg p-3 text-[12px] text-gray-600 dark:text-gray-400 whitespace-pre-wrap max-h-32 overflow-y-auto">
                {detail.output || detail.error_message || (isActive || hasActiveChildren ? (
                  <span className="inline-flex items-center gap-2 text-gray-500 dark:text-gray-400">
                    <Loader2 className="h-3 w-3 animate-spin" />
                    Waiting for active work to finish…
                  </span>
                ) : 'No output recorded.')}
              </div>
            </div>
          </div>

          {/* Trace graph — full width */}
          <div className="mb-5">
            <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300 mb-2">
              Trace
              {!isWorkflowRoot && steps.length > 0 && (
                <span className="ml-1.5 text-[10px] text-gray-400 dark:text-gray-500 font-normal">
                  {steps.length} step{steps.length !== 1 ? 's' : ''} — click nodes to inspect
                </span>
              )}
              {hasChildren && (
                <span className="ml-1.5 text-[10px] text-gray-400 dark:text-gray-500 font-normal">
                  {hasActiveChildren ? 'child runs still working below' : 'child runs available below'}
                </span>
              )}
            </p>
            <TraceGraph run={detail} steps={steps} />
          </div>

          {/* Child runs table */}
          {hasChildren && (
            <div className="mb-5">
              <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300 mb-2">
                {isWorkflowRoot ? 'Agent runs' : 'Background runs'}
                <span className="ml-1.5 text-[10px] text-gray-400 dark:text-gray-500 font-normal">
                  {children.length} run{children.length !== 1 ? 's' : ''} — expand to see output and steps
                </span>
              </p>
              <div className="bg-white dark:bg-gray-900 border border-gray-100 dark:border-gray-800 rounded-xl overflow-x-auto">
                <div className="min-w-[480px]">
                <div className={`${ROW_GRID} ${GRID_COLS} py-1.5 bg-gray-50 dark:bg-gray-800/60 border-b border-gray-100 dark:border-gray-800`}>
                  <span />
                  <span className="text-[10px] font-medium text-gray-400 dark:text-gray-500 uppercase">Agent</span>
                  <span className="text-[10px] font-medium text-gray-400 dark:text-gray-500 uppercase text-center">Status</span>
                  <span className="text-[10px] font-medium text-gray-400 dark:text-gray-500 uppercase text-right">Dur</span>
                  <span className="text-[10px] font-medium text-gray-400 dark:text-gray-500 uppercase text-right">Tokens</span>
                  <span className="text-[10px] font-medium text-gray-400 dark:text-gray-500 uppercase text-right">Cost</span>
                </div>
                {children.map((child) => (
                  <SubRunRow key={child.id} subRun={child} agentName={child.agent_id ? agentNames[child.agent_id] : undefined} />
                ))}
                </div>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}
