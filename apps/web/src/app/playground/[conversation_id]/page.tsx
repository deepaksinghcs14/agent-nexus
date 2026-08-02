'use client'

import { use, useState, useRef, useEffect, useCallback, Suspense } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'next/navigation'
import {
  AlertCircle, Bot, CheckCircle, Send, User, Wrench,
  XCircle, ChevronDown, Loader2, ExternalLink, Cpu,
} from 'lucide-react'
import { conversationsAPI, agentsAPI, runsAPI } from '@/lib/api'
import { relativeTime } from '@/lib/utils'
import { MarkdownMessage } from '@/components/MarkdownMessage'
import type { Agent, Conversation, Message } from '@/types'

interface ConvData { conversation: Conversation; messages: Message[] }
interface RunEvent {
  type: 'run_started' | 'delta' | 'run_completed' | 'tool_call' | 'tool_started' | 'approval_required' | 'user_input_required' | 'error' | 'compacting' | 'ping' | 'session_wait' | 'session_parked' | 'approval_parked'
  content?: string
  error?: string
  run_id?: string
  usage?: { output?: number }
  call_id?: string
  tool?: string
  input?: unknown
  output?: unknown
  latency_ms?: number
  approval_id?: string
  question?: string
  status?: string
}
interface TraceEvent {
  callId: string
  tool: string
  input: unknown
  output: unknown
  latencyMs: number
  error: boolean
  pending: boolean
}
interface ApprovalState {
  approvalId: string
  runId: string
  tool: string
  input: unknown
}
interface UserInputState {
  runId: string
  question: string
}

async function responseError(res: Response) {
  try { const b = await res.json(); return b.error ?? b.message ?? `${res.status}` }
  catch { return res.statusText || `${res.status}` }
}

function hasError(output: unknown): boolean {
  if (output && typeof output === 'object') {
    const o = output as Record<string, unknown>
    if ('error' in o) return true
    try {
      const s = typeof o === 'object' ? JSON.stringify(o) : String(o)
      const parsed = JSON.parse(s)
      if (parsed && typeof parsed === 'object' && 'error' in parsed) return true
    } catch { /* */ }
  }
  if (typeof output === 'string') {
    try { const p = JSON.parse(output); if (p && typeof p === 'object' && 'error' in p) return true }
    catch { /* */ }
  }
  return false
}

function ToolTrace({ traces, open }: { traces: TraceEvent[]; open?: boolean }) {
  if (traces.length === 0) return null
  const errorCount = traces.filter(t => t.error).length
  const pendingCount = traces.filter(t => t.pending).length
  return (
    <div className="ml-10 mr-4 mb-1">
      <details open={open}>
        <summary className="flex items-center gap-2 cursor-pointer list-none select-none group py-1">
          <div className="flex items-center gap-1.5">
            <Wrench size={12} className="text-accent dark:text-accent-bright" />
            <span className="text-[11px] font-medium text-muted-foreground group-hover:text-foreground dark:hover:text-faint">
              {traces.length} tool call{traces.length !== 1 ? 's' : ''}
            </span>
            {pendingCount > 0 && (
              <span className="text-[10px] text-info font-medium">· {pendingCount} running</span>
            )}
            {errorCount > 0 && (
              <span className="text-[10px] text-crit font-medium">· {errorCount} failed</span>
            )}
          </div>
          <ChevronDown size={11} className="text-faint transition-transform group-open:rotate-180 ml-0.5" />
        </summary>
        <div className="mt-1 rounded-lg border border-border bg-muted overflow-hidden">
          {traces.map((t, i) => (
            <details key={i} className="group/item border-b border-border last:border-b-0">
              <summary className="flex items-center gap-2 px-3 py-2 cursor-pointer list-none select-none hover:bg-muted/80">
                {t.pending ? (
                  <Loader2 size={12} className="text-info animate-spin flex-shrink-0" />
                ) : (
                  <span className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${t.error ? 'bg-crit' : 'bg-good'}`} />
                )}
                <span className="font-mono text-[11px] font-medium text-foreground flex-1 truncate">{t.tool}</span>
                {t.pending ? (
                  <span className="text-[10px] text-info flex-shrink-0">running…</span>
                ) : (
                  <span className="text-[10px] text-faint flex-shrink-0">{t.latencyMs}ms</span>
                )}
                <ChevronDown size={10} className="text-faint transition-transform group-open/item:rotate-180" />
              </summary>
              <div className="px-3 pb-3 pt-1 space-y-2 bg-surface">
                <div>
                  <p className="text-[10px] uppercase tracking-wide text-faint font-semibold mb-1">Input</p>
                  <pre className="text-[11px] text-muted-foreground bg-muted border border-border rounded p-2 overflow-x-auto whitespace-pre-wrap max-h-40">{JSON.stringify(t.input, null, 2)}</pre>
                </div>
                {!t.pending && (
                  <div>
                    <p className="text-[10px] uppercase tracking-wide text-faint font-semibold mb-1">Output</p>
                    <pre className={`text-[11px] bg-muted border rounded p-2 overflow-x-auto whitespace-pre-wrap max-h-40 ${t.error ? 'text-crit border-crit/30 bg-crit/10' : 'text-muted-foreground border-border'}`}>{JSON.stringify(t.output, null, 2)}</pre>
                  </div>
                )}
              </div>
            </details>
          ))}
        </div>
      </details>
    </div>
  )
}

// LiveTracePanel — the mock's right-side "Live trace" waterfall. Shows the
// current turn's tool calls as labeled bars scaled by latency, with a status dot.
function LiveTracePanel({ traces, running }: { traces: TraceEvent[]; running: boolean }) {
  const maxLatency = Math.max(1, ...traces.map(t => t.latencyMs || 0))
  const totalMs = traces.reduce((s, t) => s + (t.latencyMs || 0), 0)
  return (
    <aside className="hidden lg:flex flex-col w-80 border-l border-border bg-surface flex-shrink-0">
      <div className="flex items-center justify-between px-4 py-3 border-b border-border">
        <h3 className="text-sm font-semibold text-foreground">Live trace</h3>
        {running
          ? <span className="inline-flex items-center gap-1.5 font-mono text-[10px] text-accent dark:text-accent-bright"><span className="w-1.5 h-1.5 rounded-full bg-accent dark:bg-accent-bright animate-pulse" />running</span>
          : traces.length > 0
            ? <span className="font-mono text-[10px] text-good">done</span>
            : null}
      </div>
      <div className="flex-1 overflow-y-auto p-3">
        {traces.length === 0 ? (
          <div className="text-[12px] text-faint text-center py-10 px-4">
            Tool calls stream here as the agent runs.
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {traces.map((t) => {
              const pct = Math.max(6, ((t.latencyMs || 0) / maxLatency) * 100)
              const tone = t.error ? 'bg-crit' : t.pending ? 'bg-accent dark:bg-accent-bright' : 'bg-good'
              return (
                <div key={t.callId} className="grid grid-cols-[92px_1fr] gap-2 items-center">
                  <span className="font-mono text-[10px] text-muted-foreground truncate" title={t.tool}>{t.tool}</span>
                  <div className="h-4 rounded bg-surface-2 relative overflow-hidden">
                    <div className={`absolute inset-y-0 left-0 rounded ${tone} ${t.pending ? 'animate-pulse' : ''}`} style={{ width: `${pct}%` }} />
                    {!t.pending && (t.latencyMs || 0) > 0 && (
                      <span className="absolute right-1 top-1/2 -translate-y-1/2 font-mono text-[9px] text-muted-foreground">{(t.latencyMs / 1000).toFixed(2)}s</span>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
      {traces.length > 0 && (
        <div className="px-4 py-2.5 border-t border-border font-mono text-[10px] text-faint">
          {traces.length} tool{traces.length !== 1 ? 's' : ''} · {(totalMs / 1000).toFixed(2)}s total
        </div>
      )}
    </aside>
  )
}

function PlaygroundConversation({ params }: { params: { conversation_id: string } }) {
  const queryClient = useQueryClient()
  const searchParams = useSearchParams()
  const convId = params.conversation_id

  const [input, setInput] = useState('')
  const [messages, setMessages] = useState<Message[]>([])
  const [streaming, setStreaming] = useState(false)
  const [streamBuffer, setStreamBuffer] = useState('')
  const [thinking, setThinking] = useState(false)
  const [errorMessage, setErrorMessage] = useState('')
  const [approvalState, setApprovalState] = useState<ApprovalState | null>(null)
  const [userInputState, setUserInputState] = useState<UserInputState | null>(null)
  const [userInputAnswer, setUserInputAnswer] = useState('')
  const [submittingInput, setSubmittingInput] = useState(false)
  const [activeRunId, setActiveRunId] = useState<string | null>(null)
  const [approvingDecision, setApprovingDecision] = useState<string | null>(null)
  const [isCompacting, setIsCompacting] = useState(false)
  // Set while the run is parked (or blocked) on an external wait: an autonomous
  // coding session ('session') or a pending approval whose SSE stream has ended
  // ('approval'). The stream closes without run_completed in these cases — the
  // reply arrives later via headless resume, so we poll the run until terminal.
  const [awaitingKind, setAwaitingKind] = useState<'session' | 'approval' | null>(null)

  // Per-turn tool traces: keyed by the user message ID that triggered them
  const [turnTraces, setTurnTraces] = useState<Record<string, TraceEvent[]>>({})
  const currentUserMsgIdRef = useRef<string | null>(null)
  const activeRunIdRef = useRef<string | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['conversation', convId],
    queryFn: () => conversationsAPI.get(convId) as Promise<ConvData>,
  })

  const agentId = data?.conversation?.agent_id
  const { data: agent } = useQuery<Agent>({
    queryKey: ['agent', agentId],
    queryFn: () => agentsAPI.get(agentId!) as Promise<Agent>,
    enabled: !!agentId,
  })

  const autoSentRef = useRef(false)
  const pollAbortRef = useRef(false)
  // The current in-flight stream reader, and the auto-send effect's launch
  // timer. This component is NOT keyed by convId — a client-side nav
  // between conversations reuses this instance, so without this a stream
  // still running for the previous conversation would keep writing into
  // this one's state.
  const readerRef = useRef<ReadableStreamDefaultReader<Uint8Array> | null>(null)
  const autoSendTimer = useRef<number | null>(null)

  useEffect(() => { if (data) setMessages(data.messages ?? []) }, [data])
  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [messages, streamBuffer, thinking])
  useEffect(() => { activeRunIdRef.current = activeRunId }, [activeRunId])
  useEffect(() => {
    pollAbortRef.current = false
    return () => {
      pollAbortRef.current = true
      readerRef.current?.cancel().catch(() => {})
      readerRef.current = null
      if (autoSendTimer.current != null) window.clearTimeout(autoSendTimer.current)
    }
  }, [convId])

  // A parked run's SSE stream has ended; the run resumes headlessly when the
  // session callback / approval decision arrives. Poll until it leaves every
  // active state, then refetch the conversation to pick up the reply.
  const pollParkedRun = useCallback(async (runId: string) => {
    const ACTIVE = ['pending', 'running', 'session_wait', 'approval_wait']
    let finalStatus = ''
    for (;;) {
      await new Promise(r => setTimeout(r, 5000))
      if (pollAbortRef.current) return
      const res = await runsAPI.get(runId).catch(() => null) as { run?: { status?: string } } | null
      const status = res?.run?.status ?? ''
      if (status && !ACTIVE.includes(status)) { finalStatus = status; break }
    }
    setAwaitingKind(null)
    setApprovalState(null)
    if (finalStatus === 'failed') setErrorMessage('The run failed while waiting — open the run for details.')
    await queryClient.invalidateQueries({ queryKey: ['conversation', convId] })
    await queryClient.invalidateQueries({ queryKey: ['conversations'] })
    await queryClient.invalidateQueries({ queryKey: ['runs'] })
  }, [convId, queryClient])

  // Auto-send first message passed via ?msg= from the launch page
  useEffect(() => {
    const msg = searchParams.get('msg')
    if (msg && !autoSentRef.current && data && data.messages.length === 0) {
      autoSentRef.current = true
      setInput(msg)
      // Small delay to let the component settle before firing
      autoSendTimer.current = window.setTimeout(() => {
        setInput('')
        setThinking(true)
        const userMsgId = crypto.randomUUID()
        currentUserMsgIdRef.current = userMsgId
        const userMsg: Message = {
          id: userMsgId,
          conversation_id: convId,
          role: 'user',
          content: msg,
          tokens: 0,
          created_at: new Date().toISOString(),
        }
        setMessages([userMsg])
        setTurnTraces({ [userMsgId]: [] })
        setStreaming(true)
        setStreamBuffer('')
        const token = localStorage.getItem('access_token')
        const apiUrl = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'
        // Hoisted so the .catch() below (a separate closure from .then()) can
        // still identify which reader this chain owns.
        let reader: ReadableStreamDefaultReader<Uint8Array> | null = null
        fetch(`${apiUrl}/api/v1/conversations/${convId}/runs`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
          body: JSON.stringify({ input: msg }),
          credentials: 'include',
        }).then(async res => {
          if (!res.ok || !res.body) throw new Error(await responseError(res))
          reader = res.body.getReader()
          readerRef.current?.cancel().catch(() => {})
          readerRef.current = reader
          const decoder = new TextDecoder()
          let buf = '', full = '', completed = false, parked = false
          const appendAssistant = (tokens = 0) => {
            if (completed || !full) return
            completed = true
            setMessages(prev => [...prev, {
              id: crypto.randomUUID(), conversation_id: convId,
              role: 'assistant', content: full, tokens, created_at: new Date().toISOString(),
            }])
            setStreamBuffer('')
          }
          const processLine = (line: string) => {
            if (!line.startsWith('data: ')) return
            const event = JSON.parse(line.slice(6)) as RunEvent
            if (event.type === 'run_started') { setActiveRunId(event.run_id ?? null); activeRunIdRef.current = event.run_id ?? null }
            if (event.type === 'delta') { setThinking(false); full += event.content ?? ''; setStreamBuffer(full) }
            if (event.type === 'tool_started') {
              const uid = userMsgId
              setTurnTraces(prev => ({
                ...prev,
                [uid]: [...(prev[uid] ?? []), { callId: event.call_id ?? '', tool: event.tool ?? '', input: event.input, output: null, latencyMs: 0, error: false, pending: true }],
              }))
            }
            if (event.type === 'tool_call') {
              setThinking(false)
              setAwaitingKind(null)
              const uid = userMsgId
              const errored = hasError(event.output)
              const updated = { callId: event.call_id ?? '', tool: event.tool ?? '', input: event.input, output: event.output, latencyMs: event.latency_ms ?? 0, error: errored, pending: false }
              setTurnTraces(prev => {
                const existing = prev[uid] ?? []
                const idx = existing.findIndex(t => t.callId === event.call_id && t.pending)
                if (idx >= 0) { const next = [...existing]; next[idx] = updated; return { ...prev, [uid]: next } }
                return { ...prev, [uid]: [...existing, updated] }
              })
            }
            if (event.type === 'approval_required') setApprovalState({ approvalId: event.approval_id ?? '', runId: event.run_id ?? '', tool: event.tool ?? '', input: event.input })
            if (event.type === 'user_input_required') setUserInputState({ runId: event.run_id ?? '', question: event.question ?? '' })
            if (event.type === 'run_completed') { appendAssistant(event.usage?.output ?? 0); setApprovalState(null); setUserInputState(null); setAwaitingKind(null) }
            if (event.type === 'session_wait') { setThinking(false); setAwaitingKind('session') }
            if (event.type === 'session_parked' || event.type === 'approval_parked') {
              parked = true
              completed = true
              setAwaitingKind(event.type === 'approval_parked' ? 'approval' : 'session')
              setStreamBuffer('')
            }
            if (event.type === 'compacting') { if (event.status === 'start') setIsCompacting(true); if (event.status === 'done') setIsCompacting(false) }
            if (event.type === 'error') throw new Error(event.error ?? 'Run failed')
          }
          while (true) {
            const { done, value } = await reader.read()
            if (done) break
            buf += decoder.decode(value, { stream: true })
            const lines = buf.split('\n'); buf = lines.pop() ?? ''
            for (const line of lines) processLine(line)
          }
          buf += decoder.decode()
          for (const line of buf.split('\n')) processLine(line)
          if (readerRef.current !== reader) return   // cancelled by unmount/conversation change
          readerRef.current = null
          appendAssistant()
          if (parked && activeRunIdRef.current) void pollParkedRun(activeRunIdRef.current)
          await queryClient.invalidateQueries({ queryKey: ['conversation', convId] })
          await queryClient.invalidateQueries({ queryKey: ['conversations'] })
          await queryClient.invalidateQueries({ queryKey: ['runs'] })
        }).catch(err => {
          if (readerRef.current !== reader) return   // cancelled — not a real failure
          setErrorMessage(err instanceof Error ? err.message : 'Failed to send message')
        }).finally(() => { setStreaming(false); setThinking(false) })
      }, 100)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  // Auto-resize textarea
  const handleInput = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInput(e.target.value)
    const el = e.target
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 160) + 'px'
  }, [])

  async function sendMessage() {
    if (!input.trim() || streaming) return
    const text = input.trim()
    setInput('')
    if (textareaRef.current) { textareaRef.current.style.height = 'auto' }
    setErrorMessage('')
    setThinking(true)

    const userMsgId = crypto.randomUUID()
    currentUserMsgIdRef.current = userMsgId

    const userMsg: Message = {
      id: userMsgId,
      conversation_id: convId,
      role: 'user',
      content: text,
      tokens: 0,
      created_at: new Date().toISOString(),
    }
    setMessages(prev => [...prev, userMsg])
    setTurnTraces(prev => ({ ...prev, [userMsgId]: [] }))
    setStreaming(true)
    setStreamBuffer('')
    setApprovalState(null)
    setUserInputState(null)
    setActiveRunId(null)

    // Hoisted above the try block so the catch clause can still identify
    // which reader this call owns (a const declared inside try isn't
    // visible in catch).
    let reader: ReadableStreamDefaultReader<Uint8Array> | null = null
    try {
      const token = localStorage.getItem('access_token')
      const apiUrl = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'
      const res = await fetch(`${apiUrl}/api/v1/conversations/${convId}/runs`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({ input: text }),
        credentials: 'include',
      })

      if (!res.ok || !res.body) throw new Error(await responseError(res))

      reader = res.body.getReader()
      readerRef.current?.cancel().catch(() => {})
      readerRef.current = reader
      const decoder = new TextDecoder()
      let buf = '', full = '', completed = false, parked = false

      const appendAssistant = (tokens = 0) => {
        if (completed || !full) return
        completed = true
        setMessages(prev => [...prev, {
          id: crypto.randomUUID(),
          conversation_id: convId,
          role: 'assistant',
          content: full,
          tokens,
          created_at: new Date().toISOString(),
        }])
        setStreamBuffer('')
      }

      const processLine = (line: string) => {
        if (!line.startsWith('data: ')) return
        const event = JSON.parse(line.slice(6)) as RunEvent
        if (event.type === 'run_started') {
          const rid = event.run_id ?? null
          setActiveRunId(rid)
          activeRunIdRef.current = rid
        }
        if (event.type === 'delta') {
          setThinking(false)
          full += event.content ?? ''
          setStreamBuffer(full)
        }
        if (event.type === 'tool_started') {
          const uid = currentUserMsgIdRef.current
          if (uid) {
            setTurnTraces(prev => ({
              ...prev,
              [uid]: [...(prev[uid] ?? []), {
                callId: event.call_id ?? '',
                tool: event.tool ?? '',
                input: event.input,
                output: null,
                latencyMs: 0,
                error: false,
                pending: true,
              }],
            }))
          }
        }
        if (event.type === 'tool_call') {
          setThinking(false)
          setAwaitingKind(null)
          const uid = currentUserMsgIdRef.current
          if (uid) {
            const errored = hasError(event.output)
            const updated = {
              callId: event.call_id ?? '',
              tool: event.tool ?? '',
              input: event.input,
              output: event.output,
              latencyMs: event.latency_ms ?? 0,
              error: errored,
              pending: false,
            }
            setTurnTraces(prev => {
              const existing = prev[uid] ?? []
              const idx = existing.findIndex(t => t.callId === event.call_id && t.pending)
              if (idx >= 0) {
                const next = [...existing]
                next[idx] = updated
                return { ...prev, [uid]: next }
              }
              return { ...prev, [uid]: [...existing, updated] }
            })
          }
        }
        if (event.type === 'approval_required') {
          setApprovalState({
            approvalId: event.approval_id ?? '',
            runId: event.run_id ?? activeRunIdRef.current ?? '',
            tool: event.tool ?? '',
            input: event.input,
          })
        }
        if (event.type === 'user_input_required') {
          setUserInputState({ runId: event.run_id ?? activeRunIdRef.current ?? '', question: event.question ?? '' })
        }
        if (event.type === 'run_completed') {
          appendAssistant(event.usage?.output ?? 0)
          setApprovalState(null)
          setUserInputState(null)
          setAwaitingKind(null)
        }
        if (event.type === 'session_wait') { setThinking(false); setAwaitingKind('session') }
        if (event.type === 'session_parked' || event.type === 'approval_parked') {
          // The run persisted its wait state; the stream ends without
          // run_completed. Suppress the partial-buffer append and hand off
          // to polling — the full reply lands in the DB on headless resume.
          parked = true
          completed = true
          setAwaitingKind(event.type === 'approval_parked' ? 'approval' : 'session')
          setStreamBuffer('')
        }
        if (event.type === 'compacting') {
          if (event.status === 'start') setIsCompacting(true)
          if (event.status === 'done') setIsCompacting(false)
        }
        if (event.type === 'error') throw new Error(event.error ?? 'Run failed')
      }

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += decoder.decode(value, { stream: true })
        const lines = buf.split('\n')
        buf = lines.pop() ?? ''
        for (const line of lines) processLine(line)
      }
      buf += decoder.decode()
      for (const line of buf.split('\n')) processLine(line)
      if (readerRef.current !== reader) return   // cancelled by unmount/conversation change
      readerRef.current = null
      appendAssistant()
      if (parked && activeRunIdRef.current) void pollParkedRun(activeRunIdRef.current)
      await queryClient.invalidateQueries({ queryKey: ['conversation', convId] })
      await queryClient.invalidateQueries({ queryKey: ['conversations'] })
      await queryClient.invalidateQueries({ queryKey: ['runs'] })
    } catch (err) {
      if (readerRef.current !== reader) return   // cancelled — not a real failure
      setErrorMessage(err instanceof Error ? err.message : 'Failed to send message')
    } finally {
      setStreaming(false)
      setThinking(false)
      setIsCompacting(false)
    }
  }

  async function handleApproval(decision: 'approved' | 'rejected') {
    if (!approvalState) return
    setApprovingDecision(decision)
    try { await runsAPI.approve(approvalState.runId, { decision }); setApprovalState(null) }
    catch { /* keep UI */ }
    finally { setApprovingDecision(null) }
  }

  async function handleUserInputSubmit() {
    if (!userInputState || !userInputAnswer.trim()) return
    setSubmittingInput(true)
    try {
      await runsAPI.submitInput(userInputState.runId, { answer: userInputAnswer.trim() })
      setUserInputState(null)
      setUserInputAnswer('')
    } catch { /* keep UI */ }
    finally { setSubmittingInput(false) }
  }

  const conversation = data?.conversation
  const loadError = error instanceof Error ? error.message : ''

  // Group messages into turns for rendering
  // Each "turn" is either a standalone message or a user/assistant pair
  const renderItems: Array<
    | { kind: 'msg'; msg: Message }
    | { kind: 'trace'; userMsgId: string; traces: TraceEvent[]; isCurrent: boolean }
  > = []
  for (const msg of messages) {
    renderItems.push({ kind: 'msg', msg })
    if (msg.role === 'user') {
      const traces = turnTraces[msg.id]
      const isCurrent = msg.id === currentUserMsgIdRef.current
      if (traces && (traces.length > 0 || isCurrent)) {
        renderItems.push({ kind: 'trace', userMsgId: msg.id, traces, isCurrent })
      }
    }
  }

  const modelLabel = agent
    ? agent.model.split('/').pop()?.split('-').slice(0, 3).join('-') ?? agent.model
    : null

  // Traces for the live-trace side panel: the most recent turn that has any.
  const panelTraces = (() => {
    const ids = Object.keys(turnTraces)
    for (let i = ids.length - 1; i >= 0; i--) {
      if (turnTraces[ids[i]]?.length) return turnTraces[ids[i]]
    }
    return [] as TraceEvent[]
  })()

  return (
    <div className="flex flex-col h-full bg-surface">
      {/* Header */}
      <div className="border-b border-border px-5 py-3 flex items-center gap-3 flex-shrink-0">
        <div className="w-8 h-8 rounded-lg bg-accent/15 flex items-center justify-center flex-shrink-0">
          <Bot size={16} className="text-accent dark:text-accent-bright" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-sm font-semibold text-foreground truncate">
              {agent?.name ?? conversation?.title ?? 'Loading…'}
            </span>
            {modelLabel && (
              <span className="flex items-center gap-1 text-[10px] font-medium text-accent dark:text-accent-bright bg-accent/10 border border-accent/25 rounded-full px-2 py-0.5">
                <Cpu size={9} /> {modelLabel}
              </span>
            )}
          </div>
          {conversation?.title && agent?.name && (
            <p className="text-[11px] text-faint truncate">{conversation.title}</p>
          )}
        </div>
        {activeRunId && (
          <a
            href={`/runs/${activeRunId}`}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-1 text-[11px] text-accent dark:text-accent-bright hover:text-accent dark:text-accent-bright flex-shrink-0"
          >
            View run <ExternalLink size={11} />
          </a>
        )}
      </div>

      {/* Chat + live trace panel */}
      <div className="flex flex-1 min-h-0 overflow-hidden">
      <div className="flex flex-col flex-1 min-w-0">
      {/* Messages */}
      <div className="flex-1 overflow-y-auto px-4 py-5 space-y-3">
        {isLoading && <div className="text-sm text-faint text-center py-12">Loading…</div>}

        {loadError && (
          <div className="flex items-center justify-between gap-3 rounded-lg border border-crit/30 bg-crit/10 px-3 py-2 text-sm text-crit dark:text-crit">
            <span className="flex items-center gap-2"><AlertCircle size={14} />{loadError}</span>
            <button onClick={() => refetch()} className="text-xs font-medium hover:underline">Retry</button>
          </div>
        )}

        {!isLoading && messages.length === 0 && !streaming && (
          <div className="text-center py-16">
            <div className="w-12 h-12 rounded-full bg-accent/15 flex items-center justify-center mx-auto mb-3">
              <Bot size={22} className="text-accent dark:text-accent-bright" />
            </div>
            <p className="text-sm font-medium text-foreground">{agent?.name ?? 'Agent'}</p>
            <p className="text-xs text-faint mt-1">Send a message to start the conversation.</p>
          </div>
        )}

        {renderItems.map((item, i) => {
          if (item.kind === 'trace') {
            return (
              <ToolTrace
                key={`trace-${item.userMsgId}`}
                traces={item.traces}
                open={item.isCurrent}
              />
            )
          }

          const msg = item.msg
          const isUser = msg.role === 'user'
          return (
            <div key={msg.id} className={`flex gap-2.5 ${isUser ? 'justify-end' : 'justify-start'}`}>
              {!isUser && (
                <div className="w-7 h-7 rounded-full bg-accent/15 flex items-center justify-center flex-shrink-0 mt-0.5">
                  <Bot size={13} className="text-accent dark:text-accent-bright" />
                </div>
              )}
              <div className={`max-w-[90%] sm:max-w-[80%] rounded-2xl px-4 py-2.5 text-sm leading-relaxed ${
                isUser
                  ? 'bg-accent text-white rounded-br-sm'
                  : 'bg-muted border border-border text-foreground rounded-bl-sm'
              }`}>
                {isUser ? (
                  <p className="whitespace-pre-wrap">{msg.content}</p>
                ) : (
                  <MarkdownMessage content={msg.content} />
                )}
                <p className={`text-[10px] mt-1.5 ${isUser ? 'text-accent-bright' : 'text-faint'}`}>
                  {relativeTime(msg.created_at)}
                </p>
              </div>
              {isUser && (
                <div className="w-7 h-7 rounded-full bg-muted flex items-center justify-center flex-shrink-0 mt-0.5">
                  <User size={13} className="text-muted-foreground" />
                </div>
              )}
            </div>
          )
        })}

        {/* Thinking indicator — before first delta */}
        {thinking && (
          <div className="flex gap-2.5 justify-start">
            <div className="w-7 h-7 rounded-full bg-accent/15 flex items-center justify-center flex-shrink-0">
              <Bot size={13} className="text-accent dark:text-accent-bright" />
            </div>
            <div className="bg-muted border border-border rounded-2xl rounded-bl-sm px-4 py-3 flex items-center gap-1.5">
              <span className="w-1.5 h-1.5 bg-faint rounded-full animate-bounce [animation-delay:0ms]" />
              <span className="w-1.5 h-1.5 bg-faint rounded-full animate-bounce [animation-delay:150ms]" />
              <span className="w-1.5 h-1.5 bg-faint rounded-full animate-bounce [animation-delay:300ms]" />
            </div>
          </div>
        )}

        {/* Streaming response */}
        {streamBuffer && (
          <div className="flex gap-2.5 justify-start">
            <div className="w-7 h-7 rounded-full bg-accent/15 flex items-center justify-center flex-shrink-0 mt-0.5">
              <Bot size={13} className="text-accent dark:text-accent-bright" />
            </div>
            <div className="max-w-[90%] sm:max-w-[80%] rounded-2xl rounded-bl-sm px-4 py-2.5 text-sm leading-relaxed bg-muted border border-border text-foreground">
              <MarkdownMessage content={streamBuffer} isStreaming />
              <span className="inline-block w-1.5 h-3.5 bg-faint ml-0.5 animate-pulse rounded-sm" />
            </div>
          </div>
        )}

        {/* Approval gate */}
        {approvalState && (
          <div className="flex gap-2.5 justify-start">
            <div className="w-7 h-7 rounded-full bg-warn/15 flex items-center justify-center flex-shrink-0 mt-0.5">
              <Bot size={13} className="text-warn dark:text-warn" />
            </div>
            <div className="max-w-[90%] sm:max-w-[80%] rounded-2xl border border-warn/30 bg-warn/10 px-4 py-3">
              <p className="text-xs font-semibold text-warn mb-1">Approval required</p>
              <p className="text-[12px] text-warn mb-2">
                Tool <code className="font-mono bg-warn/15 px-1 rounded">{approvalState.tool}</code> wants to run with:
              </p>
              <pre className="text-[11px] text-warn bg-surface border border-warn/30 rounded p-2 overflow-x-auto whitespace-pre-wrap mb-3 max-h-32">{JSON.stringify(approvalState.input, null, 2)}</pre>
              <div className="flex gap-2">
                <button onClick={() => handleApproval('approved')} disabled={!!approvingDecision}
                  className="flex items-center gap-1.5 px-3 py-1.5 bg-good text-white rounded-lg text-[12px] font-medium hover:bg-good disabled:opacity-50">
                  <CheckCircle size={12} /> Approve
                </button>
                <button onClick={() => handleApproval('rejected')} disabled={!!approvingDecision}
                  className="flex items-center gap-1.5 px-3 py-1.5 bg-crit/100 text-white rounded-lg text-[12px] font-medium hover:bg-crit disabled:opacity-50">
                  <XCircle size={12} /> Reject
                </button>
              </div>
            </div>
          </div>
        )}

        {/* User input gate */}
        {userInputState && (
          <div className="flex gap-2.5 justify-start">
            <div className="w-7 h-7 rounded-full bg-info/15 flex items-center justify-center flex-shrink-0 mt-0.5">
              <Bot size={13} className="text-info" />
            </div>
            <div className="max-w-[90%] sm:max-w-[80%] rounded-2xl border border-info/30 bg-info/10 px-4 py-3 w-full sm:w-80">
              <p className="text-xs font-semibold text-info dark:text-info mb-2">Agent is asking</p>
              <p className="text-[13px] text-info mb-3">{userInputState.question}</p>
              <textarea
                value={userInputAnswer}
                onChange={e => setUserInputAnswer(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleUserInputSubmit() } }}
                placeholder="Type your answer…"
                rows={2}
                className="w-full text-sm bg-surface border border-info/30 rounded-lg px-3 py-2 focus:outline-none focus:border-info/50 resize-none mb-2"
              />
              <button
                onClick={handleUserInputSubmit}
                disabled={submittingInput || !userInputAnswer.trim()}
                className="flex items-center gap-1.5 px-3 py-1.5 bg-info text-white rounded-lg text-[12px] font-medium hover:bg-info disabled:opacity-50"
              >
                <Send size={12} /> Send reply
              </button>
            </div>
          </div>
        )}

        {/* External-wait indicator: run parked on a coding session or approval */}
        {awaitingKind && (
          <div className="flex gap-2.5 justify-start">
            <div className="w-7 h-7 rounded-full bg-info/15 flex items-center justify-center flex-shrink-0 mt-0.5">
              <Bot size={13} className="text-info" />
            </div>
            <div className="max-w-[90%] sm:max-w-[80%] rounded-2xl border border-info/30 bg-info/10 px-4 py-3">
              <p className="text-xs font-semibold text-info mb-1 flex items-center gap-1.5">
                <Loader2 size={12} className="animate-spin" />
                {awaitingKind === 'session' ? 'Coding session in progress' : 'Waiting for approval'}
              </p>
              <p className="text-[12px] text-info">
                {awaitingKind === 'session'
                  ? 'The agent launched an autonomous repo session and is waiting for it to finish — this can take minutes to hours. The reply will appear here automatically.'
                  : 'The run is paused until the pending tool call above is approved or rejected. It resumes automatically after your decision.'}
              </p>
            </div>
          </div>
        )}

        {/* Compacting indicator */}
        {isCompacting && (
          <div className="flex items-center gap-1.5 px-3 py-1 text-[11px] text-faint">
            <span className="inline-block w-1.5 h-1.5 rounded-full bg-faint animate-pulse" />
            Compacting conversation…
          </div>
        )}

        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div className="border-t border-border px-4 py-3 flex-shrink-0 bg-surface">
        {errorMessage && (
          <div className="mb-2 flex items-center gap-2 rounded-lg border border-crit/30 bg-crit/10 px-3 py-2 text-sm text-crit dark:text-crit">
            <AlertCircle size={13} /><span>{errorMessage}</span>
          </div>
        )}
        <div className="flex items-end gap-2 rounded-xl border border-border-strong bg-muted px-3 py-2 focus-within:border-accent/50 focus-within:bg-white transition-colors">
          <textarea
            ref={textareaRef}
            value={input}
            onChange={handleInput}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage() } }}
            placeholder={streaming ? 'Agent is running…' : awaitingKind ? 'Waiting for the current run to resume…' : 'Message the agent… (Enter to send, Shift+Enter for newline)'}
            rows={1}
            disabled={streaming || !!awaitingKind}
            className="flex-1 text-sm bg-transparent resize-none focus:outline-none disabled:opacity-50 placeholder-faint"
            style={{ minHeight: 24, maxHeight: 160 }}
          />
          <button
            onClick={sendMessage}
            disabled={!input.trim() || streaming || !!awaitingKind}
            className="flex-shrink-0 w-8 h-8 flex items-center justify-center rounded-lg bg-accent text-white hover:bg-accent-hover disabled:opacity-40 transition-colors mb-0.5"
          >
            {streaming ? <Loader2 size={14} className="animate-spin" /> : <Send size={14} />}
          </button>
        </div>
        <p className="text-[10px] text-faint mt-1.5 text-right">
          {messages.length} message{messages.length !== 1 ? 's' : ''}
          {agent && <> · {agent.max_steps} max steps</>}
        </p>
      </div>
      </div>
      <LiveTracePanel traces={panelTraces} running={streaming || !!awaitingKind} />
      </div>
    </div>
  )
}

export default function PlaygroundConversationPage({ params }: { params: Promise<{ conversation_id: string }> }) {
  const p = use(params)
  return (
    <Suspense fallback={<div className="flex items-center justify-center h-full text-sm text-faint">Loading…</div>}>
      <PlaygroundConversation params={p} />
    </Suspense>
  )
}
