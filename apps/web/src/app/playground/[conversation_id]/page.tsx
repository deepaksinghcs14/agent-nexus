'use client'

import { useState, useRef, useEffect, useCallback, Suspense } from 'react'
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
  type: 'run_started' | 'delta' | 'run_completed' | 'tool_call' | 'approval_required' | 'error'
  content?: string
  error?: string
  run_id?: string
  usage?: { output?: number }
  tool?: string
  input?: unknown
  output?: unknown
  latency_ms?: number
  approval_id?: string
}
interface TraceEvent {
  tool: string
  input: unknown
  output: unknown
  latencyMs: number
  error: boolean
}
interface ApprovalState {
  approvalId: string
  runId: string
  tool: string
  input: unknown
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
  return (
    <div className="ml-10 mr-4 mb-1">
      <details open={open}>
        <summary className="flex items-center gap-2 cursor-pointer list-none select-none group py-1">
          <div className="flex items-center gap-1.5">
            <Wrench size={12} className="text-purple-400" />
            <span className="text-[11px] font-medium text-gray-500 group-hover:text-gray-700">
              {traces.length} tool call{traces.length !== 1 ? 's' : ''}
            </span>
            {errorCount > 0 && (
              <span className="text-[10px] text-red-500 font-medium">· {errorCount} failed</span>
            )}
          </div>
          <ChevronDown size={11} className="text-gray-400 transition-transform group-open:rotate-180 ml-0.5" />
        </summary>
        <div className="mt-1 rounded-lg border border-gray-100 bg-gray-50 overflow-hidden">
          {traces.map((t, i) => (
            <details key={i} className="group/item border-b border-gray-100 last:border-b-0">
              <summary className="flex items-center gap-2 px-3 py-2 cursor-pointer list-none select-none hover:bg-gray-100/80">
                <span className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${t.error ? 'bg-red-400' : 'bg-green-400'}`} />
                <span className="font-mono text-[11px] font-medium text-gray-700 flex-1 truncate">{t.tool}</span>
                <span className="text-[10px] text-gray-400 flex-shrink-0">{t.latencyMs}ms</span>
                <ChevronDown size={10} className="text-gray-400 transition-transform group-open/item:rotate-180" />
              </summary>
              <div className="px-3 pb-3 pt-1 space-y-2 bg-white">
                <div>
                  <p className="text-[10px] uppercase tracking-wide text-gray-400 font-semibold mb-1">Input</p>
                  <pre className="text-[11px] text-gray-600 bg-gray-50 border border-gray-100 rounded p-2 overflow-x-auto whitespace-pre-wrap max-h-40">{JSON.stringify(t.input, null, 2)}</pre>
                </div>
                <div>
                  <p className="text-[10px] uppercase tracking-wide text-gray-400 font-semibold mb-1">Output</p>
                  <pre className={`text-[11px] bg-gray-50 border rounded p-2 overflow-x-auto whitespace-pre-wrap max-h-40 ${t.error ? 'text-red-600 border-red-100 bg-red-50' : 'text-gray-600 border-gray-100'}`}>{JSON.stringify(t.output, null, 2)}</pre>
                </div>
              </div>
            </details>
          ))}
        </div>
      </details>
    </div>
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
  const [activeRunId, setActiveRunId] = useState<string | null>(null)
  const [approvingDecision, setApprovingDecision] = useState<string | null>(null)

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

  useEffect(() => { if (data) setMessages(data.messages ?? []) }, [data])
  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [messages, streamBuffer, thinking])
  useEffect(() => { activeRunIdRef.current = activeRunId }, [activeRunId])

  // Auto-send first message passed via ?msg= from the launch page
  useEffect(() => {
    const msg = searchParams.get('msg')
    if (msg && !autoSentRef.current && data && data.messages.length === 0) {
      autoSentRef.current = true
      setInput(msg)
      // Small delay to let the component settle before firing
      setTimeout(() => {
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
          const reader = res.body.getReader()
          const decoder = new TextDecoder()
          let buf = '', full = '', completed = false
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
            if (event.type === 'tool_call') {
              setThinking(false)
              const uid = userMsgId
              const errored = hasError(event.output)
              setTurnTraces(prev => ({ ...prev, [uid]: [...(prev[uid] ?? []), { tool: event.tool ?? '', input: event.input, output: event.output, latencyMs: event.latency_ms ?? 0, error: errored }] }))
            }
            if (event.type === 'approval_required') setApprovalState({ approvalId: event.approval_id ?? '', runId: event.run_id ?? '', tool: event.tool ?? '', input: event.input })
            if (event.type === 'run_completed') { appendAssistant(event.usage?.output ?? 0); setApprovalState(null) }
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
          appendAssistant()
          await queryClient.invalidateQueries({ queryKey: ['conversation', convId] })
          await queryClient.invalidateQueries({ queryKey: ['conversations'] })
          await queryClient.invalidateQueries({ queryKey: ['runs'] })
        }).catch(err => {
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
    setActiveRunId(null)

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

      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buf = '', full = '', completed = false

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
        if (event.type === 'tool_call') {
          setThinking(false)
          const uid = currentUserMsgIdRef.current
          if (uid) {
            const errored = hasError(event.output)
            setTurnTraces(prev => ({
              ...prev,
              [uid]: [...(prev[uid] ?? []), {
                tool: event.tool ?? '',
                input: event.input,
                output: event.output,
                latencyMs: event.latency_ms ?? 0,
                error: errored,
              }],
            }))
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
        if (event.type === 'run_completed') {
          appendAssistant(event.usage?.output ?? 0)
          setApprovalState(null)
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
      appendAssistant()
      await queryClient.invalidateQueries({ queryKey: ['conversation', convId] })
      await queryClient.invalidateQueries({ queryKey: ['conversations'] })
      await queryClient.invalidateQueries({ queryKey: ['runs'] })
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Failed to send message')
    } finally {
      setStreaming(false)
      setThinking(false)
    }
  }

  async function handleApproval(decision: 'approved' | 'rejected') {
    if (!approvalState) return
    setApprovingDecision(decision)
    try { await runsAPI.approve(approvalState.runId, { decision }); setApprovalState(null) }
    catch { /* keep UI */ }
    finally { setApprovingDecision(null) }
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

  return (
    <div className="flex flex-col h-full bg-white">
      {/* Header */}
      <div className="border-b border-gray-100 px-5 py-3 flex items-center gap-3 flex-shrink-0">
        <div className="w-8 h-8 rounded-lg bg-purple-100 flex items-center justify-center flex-shrink-0">
          <Bot size={16} className="text-purple-600" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-sm font-semibold text-gray-900 truncate">
              {agent?.name ?? conversation?.title ?? 'Loading…'}
            </span>
            {modelLabel && (
              <span className="flex items-center gap-1 text-[10px] font-medium text-purple-600 bg-purple-50 border border-purple-100 rounded-full px-2 py-0.5">
                <Cpu size={9} /> {modelLabel}
              </span>
            )}
          </div>
          {conversation?.title && agent?.name && (
            <p className="text-[11px] text-gray-400 truncate">{conversation.title}</p>
          )}
        </div>
        {activeRunId && (
          <a
            href={`/runs/${activeRunId}`}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-1 text-[11px] text-purple-500 hover:text-purple-700 flex-shrink-0"
          >
            View run <ExternalLink size={11} />
          </a>
        )}
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto px-4 py-5 space-y-3">
        {isLoading && <div className="text-sm text-gray-400 text-center py-12">Loading…</div>}

        {loadError && (
          <div className="flex items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            <span className="flex items-center gap-2"><AlertCircle size={14} />{loadError}</span>
            <button onClick={() => refetch()} className="text-xs font-medium hover:underline">Retry</button>
          </div>
        )}

        {!isLoading && messages.length === 0 && !streaming && (
          <div className="text-center py-16">
            <div className="w-12 h-12 rounded-full bg-purple-100 flex items-center justify-center mx-auto mb-3">
              <Bot size={22} className="text-purple-500" />
            </div>
            <p className="text-sm font-medium text-gray-700">{agent?.name ?? 'Agent'}</p>
            <p className="text-xs text-gray-400 mt-1">Send a message to start the conversation.</p>
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
                <div className="w-7 h-7 rounded-full bg-purple-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                  <Bot size={13} className="text-purple-600" />
                </div>
              )}
              <div className={`max-w-[80%] rounded-2xl px-4 py-2.5 text-sm leading-relaxed ${
                isUser
                  ? 'bg-purple-600 text-white rounded-br-sm'
                  : 'bg-gray-50 border border-gray-100 text-gray-800 rounded-bl-sm'
              }`}>
                {isUser ? (
                  <p className="whitespace-pre-wrap">{msg.content}</p>
                ) : (
                  <MarkdownMessage content={msg.content} />
                )}
                <p className={`text-[10px] mt-1.5 ${isUser ? 'text-purple-200' : 'text-gray-400'}`}>
                  {relativeTime(msg.created_at)}
                </p>
              </div>
              {isUser && (
                <div className="w-7 h-7 rounded-full bg-gray-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                  <User size={13} className="text-gray-500" />
                </div>
              )}
            </div>
          )
        })}

        {/* Thinking indicator — before first delta */}
        {thinking && (
          <div className="flex gap-2.5 justify-start">
            <div className="w-7 h-7 rounded-full bg-purple-100 flex items-center justify-center flex-shrink-0">
              <Bot size={13} className="text-purple-600" />
            </div>
            <div className="bg-gray-50 border border-gray-100 rounded-2xl rounded-bl-sm px-4 py-3 flex items-center gap-1.5">
              <span className="w-1.5 h-1.5 bg-gray-400 rounded-full animate-bounce [animation-delay:0ms]" />
              <span className="w-1.5 h-1.5 bg-gray-400 rounded-full animate-bounce [animation-delay:150ms]" />
              <span className="w-1.5 h-1.5 bg-gray-400 rounded-full animate-bounce [animation-delay:300ms]" />
            </div>
          </div>
        )}

        {/* Streaming response */}
        {streamBuffer && (
          <div className="flex gap-2.5 justify-start">
            <div className="w-7 h-7 rounded-full bg-purple-100 flex items-center justify-center flex-shrink-0 mt-0.5">
              <Bot size={13} className="text-purple-600" />
            </div>
            <div className="max-w-[80%] rounded-2xl rounded-bl-sm px-4 py-2.5 text-sm leading-relaxed bg-gray-50 border border-gray-100 text-gray-800">
              <MarkdownMessage content={streamBuffer} isStreaming />
              <span className="inline-block w-1.5 h-3.5 bg-gray-400 ml-0.5 animate-pulse rounded-sm" />
            </div>
          </div>
        )}

        {/* Approval gate */}
        {approvalState && (
          <div className="flex gap-2.5 justify-start">
            <div className="w-7 h-7 rounded-full bg-amber-100 flex items-center justify-center flex-shrink-0 mt-0.5">
              <Bot size={13} className="text-amber-600" />
            </div>
            <div className="max-w-[80%] rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3">
              <p className="text-xs font-semibold text-amber-800 mb-1">Approval required</p>
              <p className="text-[12px] text-amber-700 mb-2">
                Tool <code className="font-mono bg-amber-100 px-1 rounded">{approvalState.tool}</code> wants to run with:
              </p>
              <pre className="text-[11px] text-amber-800 bg-white border border-amber-100 rounded p-2 overflow-x-auto whitespace-pre-wrap mb-3 max-h-32">{JSON.stringify(approvalState.input, null, 2)}</pre>
              <div className="flex gap-2">
                <button onClick={() => handleApproval('approved')} disabled={!!approvingDecision}
                  className="flex items-center gap-1.5 px-3 py-1.5 bg-green-600 text-white rounded-lg text-[12px] font-medium hover:bg-green-700 disabled:opacity-50">
                  <CheckCircle size={12} /> Approve
                </button>
                <button onClick={() => handleApproval('rejected')} disabled={!!approvingDecision}
                  className="flex items-center gap-1.5 px-3 py-1.5 bg-red-500 text-white rounded-lg text-[12px] font-medium hover:bg-red-600 disabled:opacity-50">
                  <XCircle size={12} /> Reject
                </button>
              </div>
            </div>
          </div>
        )}

        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div className="border-t border-gray-100 px-4 py-3 flex-shrink-0 bg-white">
        {errorMessage && (
          <div className="mb-2 flex items-center gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            <AlertCircle size={13} /><span>{errorMessage}</span>
          </div>
        )}
        <div className="flex items-end gap-2 rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 focus-within:border-purple-300 focus-within:bg-white transition-colors">
          <textarea
            ref={textareaRef}
            value={input}
            onChange={handleInput}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage() } }}
            placeholder={streaming ? 'Agent is running…' : 'Message the agent… (Enter to send, Shift+Enter for newline)'}
            rows={1}
            disabled={streaming}
            className="flex-1 text-sm bg-transparent resize-none focus:outline-none disabled:opacity-50 placeholder-gray-400"
            style={{ minHeight: 24, maxHeight: 160 }}
          />
          <button
            onClick={sendMessage}
            disabled={!input.trim() || streaming}
            className="flex-shrink-0 w-8 h-8 flex items-center justify-center rounded-lg bg-purple-600 text-white hover:bg-purple-700 disabled:opacity-40 transition-colors mb-0.5"
          >
            {streaming ? <Loader2 size={14} className="animate-spin" /> : <Send size={14} />}
          </button>
        </div>
        <p className="text-[10px] text-gray-400 mt-1.5 text-right">
          {messages.length} message{messages.length !== 1 ? 's' : ''}
          {agent && <> · {agent.max_steps} max steps</>}
        </p>
      </div>
    </div>
  )
}

export default function PlaygroundConversationPage({ params }: { params: { conversation_id: string } }) {
  return (
    <Suspense fallback={<div className="flex items-center justify-center h-full text-sm text-gray-400">Loading…</div>}>
      <PlaygroundConversation params={params} />
    </Suspense>
  )
}
