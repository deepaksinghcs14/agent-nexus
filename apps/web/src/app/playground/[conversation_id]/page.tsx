'use client'

import { useState, useRef, useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertCircle, Bot, CheckCircle, Send, User, Wrench, XCircle } from 'lucide-react'
import { conversationsAPI, runsAPI } from '@/lib/api'
import { relativeTime } from '@/lib/utils'
import { MarkdownMessage } from '@/components/MarkdownMessage'
import type { Conversation, Message } from '@/types'

interface ConvData { conversation: Conversation; messages: Message[] }
interface RunEvent {
  type: 'run_started' | 'delta' | 'run_completed' | 'tool_call' | 'approval_required' | 'error'
  content?: string
  error?: string
  run_id?: string
  usage?: { output?: number }
  // tool_call
  tool?: string
  input?: unknown
  output?: unknown
  latency_ms?: number
  // approval_required
  approval_id?: string
}

interface TraceEvent {
  tool: string
  input: unknown
  output: unknown
  latencyMs: number
}

interface ApprovalState {
  approvalId: string
  runId: string
  tool: string
  input: unknown
}

async function responseError(res: Response) {
  const fallback = `Request failed with ${res.status}`
  try {
    const body = await res.json()
    return body.error ?? body.message ?? fallback
  } catch {
    return res.statusText || fallback
  }
}

export default function PlaygroundConversationPage({ params }: { params: { conversation_id: string } }) {
  const queryClient = useQueryClient()
  const [input, setInput] = useState('')
  const [messages, setMessages] = useState<Message[]>([])
  const [streaming, setStreaming] = useState(false)
  const [streamBuffer, setStreamBuffer] = useState('')
  const [errorMessage, setErrorMessage] = useState('')
  const [traceEvents, setTraceEvents] = useState<TraceEvent[]>([])
  const [approvalState, setApprovalState] = useState<ApprovalState | null>(null)
  const [activeRunId, setActiveRunId] = useState<string | null>(null)
  const [approvingDecision, setApprovingDecision] = useState<string | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const activeRunIdRef = useRef<string | null>(null)
  const convId = params.conversation_id

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['conversation', convId],
    queryFn: () => conversationsAPI.get(convId) as Promise<ConvData>,
  })

  useEffect(() => {
    if (data) setMessages(data.messages ?? [])
  }, [data])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, streamBuffer])

  useEffect(() => { activeRunIdRef.current = activeRunId }, [activeRunId])

  async function sendMessage() {
    if (!input.trim() || streaming) return
    const text = input.trim()
    setInput('')
    setErrorMessage('')

    const userMsg: Message = {
      id: crypto.randomUUID(),
      conversation_id: convId,
      role: 'user',
      content: text,
      tokens: 0,
      created_at: new Date().toISOString(),
    }
    setMessages((prev) => [...prev, userMsg])
    setStreaming(true)
    setStreamBuffer('')
    setTraceEvents([])
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

      if (!res.ok || !res.body) {
        throw new Error(await responseError(res))
      }

      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      let full = ''
      let completed = false

      const appendAssistant = (tokens = 0) => {
        if (completed || !full) return
        completed = true
        const assistantMsg: Message = {
          id: crypto.randomUUID(),
          conversation_id: convId,
          role: 'assistant',
          content: full,
          tokens,
          created_at: new Date().toISOString(),
        }
        setMessages((prev) => [...prev, assistantMsg])
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
          full += event.content ?? ''
          setStreamBuffer(full)
        }
        if (event.type === 'tool_call') {
          setTraceEvents((prev) => [...prev, {
            tool: event.tool ?? '',
            input: event.input,
            output: event.output,
            latencyMs: event.latency_ms ?? 0,
          }])
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
        if (event.type === 'error') {
          throw new Error(event.error ?? 'Run failed')
        }
      }

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() ?? ''
        for (const line of lines) processLine(line)
      }

      buffer += decoder.decode()
      for (const line of buffer.split('\n')) processLine(line)
      appendAssistant()
      await queryClient.invalidateQueries({ queryKey: ['conversation', convId] })
      await queryClient.invalidateQueries({ queryKey: ['conversations'] })
      await queryClient.invalidateQueries({ queryKey: ['runs'] })
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Failed to send message')
    } finally {
      setStreaming(false)
    }
  }

  async function handleApproval(decision: 'approved' | 'rejected') {
    if (!approvalState) return
    setApprovingDecision(decision)
    try {
      await runsAPI.approve(approvalState.runId, { decision })
      setApprovalState(null)
    } catch {
      // keep approval UI visible if the call fails
    } finally {
      setApprovingDecision(null)
    }
  }

  const conversation = data?.conversation
  const loadError = error instanceof Error ? error.message : ''

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="border-b border-gray-100 px-6 py-3 flex items-center justify-between flex-shrink-0">
        <div>
          <p className="text-sm font-medium text-gray-900">{conversation?.title ?? 'Loading…'}</p>
          <p className="text-xs text-gray-400 mt-0.5">{messages.length} messages</p>
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto px-6 py-4 space-y-4">
        {isLoading && <div className="text-sm text-gray-400 text-center py-8">Loading…</div>}

        {loadError && (
          <div className="flex items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            <span className="flex items-center gap-2"><AlertCircle size={15} /> {loadError}</span>
            <button onClick={() => refetch()} className="text-xs font-medium text-red-700 hover:underline">Retry</button>
          </div>
        )}

        {!isLoading && messages.length === 0 && !streaming && (
          <div className="text-center py-12">
            <Bot size={32} className="mx-auto text-gray-300 mb-3" />
            <p className="text-gray-500 text-sm">Send a message to start the conversation.</p>
          </div>
        )}

        {messages.map((msg) => (
          <div key={msg.id} className={`flex gap-3 ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
            {msg.role !== 'user' && (
              <div className="w-7 h-7 rounded-full bg-purple-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                <Bot size={14} className="text-purple-600" />
              </div>
            )}
            <div className={`max-w-[75%] rounded-xl px-4 py-2.5 text-sm ${
              msg.role === 'user'
                ? 'bg-purple-600 text-white'
                : 'bg-gray-50 border border-gray-100 text-gray-800'
            }`}>
              {msg.role === 'user' ? (
                <p className="whitespace-pre-wrap">{msg.content}</p>
              ) : (
                <MarkdownMessage content={msg.content} />
              )}
              <p className={`text-[10px] mt-1 ${msg.role === 'user' ? 'text-purple-200' : 'text-gray-400'}`}>
                {relativeTime(msg.created_at)}
              </p>
            </div>
            {msg.role === 'user' && (
              <div className="w-7 h-7 rounded-full bg-gray-200 flex items-center justify-center flex-shrink-0 mt-0.5">
                <User size={14} className="text-gray-500" />
              </div>
            )}
          </div>
        ))}

        {/* Tool call trace */}
        {traceEvents.length > 0 && (
          <div className="flex gap-3 justify-start">
            <div className="w-7 h-7 rounded-full bg-purple-100 flex items-center justify-center flex-shrink-0 mt-0.5">
              <Bot size={14} className="text-purple-600" />
            </div>
            <div className="max-w-[75%] w-full rounded-xl border border-gray-100 bg-gray-50 text-sm overflow-hidden">
              {traceEvents.map((t, i) => (
                <details key={i} className="group border-b border-gray-100 last:border-b-0">
                  <summary className="flex items-center gap-2 px-3 py-2 cursor-pointer list-none select-none hover:bg-gray-100">
                    <Wrench size={13} className="text-purple-500 flex-shrink-0" />
                    <span className="font-medium text-gray-700 text-[12px]">{t.tool}</span>
                    <span className="ml-auto text-[11px] text-gray-400">{t.latencyMs}ms</span>
                  </summary>
                  <div className="px-3 pb-2 space-y-1">
                    <p className="text-[11px] text-gray-500 font-medium">Input</p>
                    <pre className="text-[11px] text-gray-600 bg-white border border-gray-100 rounded p-2 overflow-x-auto whitespace-pre-wrap">{JSON.stringify(t.input, null, 2)}</pre>
                    <p className="text-[11px] text-gray-500 font-medium mt-1">Output</p>
                    <pre className="text-[11px] text-gray-600 bg-white border border-gray-100 rounded p-2 overflow-x-auto whitespace-pre-wrap">{JSON.stringify(t.output, null, 2)}</pre>
                  </div>
                </details>
              ))}
            </div>
          </div>
        )}

        {/* Approval gate */}
        {approvalState && (
          <div className="flex gap-3 justify-start">
            <div className="w-7 h-7 rounded-full bg-purple-100 flex items-center justify-center flex-shrink-0 mt-0.5">
              <Bot size={14} className="text-purple-600" />
            </div>
            <div className="max-w-[75%] rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm">
              <p className="font-medium text-amber-800 mb-1">Approval required</p>
              <p className="text-amber-700 text-[12px] mb-2">
                Tool <code className="font-mono bg-amber-100 px-1 rounded">{approvalState.tool}</code> requires your approval before it can run.
              </p>
              <pre className="text-[11px] text-amber-800 bg-white border border-amber-100 rounded p-2 overflow-x-auto whitespace-pre-wrap mb-3">{JSON.stringify(approvalState.input, null, 2)}</pre>
              <div className="flex gap-2">
                <button
                  onClick={() => handleApproval('approved')}
                  disabled={!!approvingDecision}
                  className="flex items-center gap-1.5 px-3 py-1.5 bg-green-600 text-white rounded-md text-[12px] font-medium hover:bg-green-700 disabled:opacity-50"
                >
                  <CheckCircle size={13} /> Approve
                </button>
                <button
                  onClick={() => handleApproval('rejected')}
                  disabled={!!approvingDecision}
                  className="flex items-center gap-1.5 px-3 py-1.5 bg-red-600 text-white rounded-md text-[12px] font-medium hover:bg-red-700 disabled:opacity-50"
                >
                  <XCircle size={13} /> Reject
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Streaming bubble — plain text during streaming to avoid layout thrashing */}
        {streamBuffer && (
          <div className="flex gap-3 justify-start">
            <div className="w-7 h-7 rounded-full bg-purple-100 flex items-center justify-center flex-shrink-0 mt-0.5">
              <Bot size={14} className="text-purple-600" />
            </div>
            <div className="max-w-[75%] rounded-xl px-4 py-2.5 text-sm bg-gray-50 border border-gray-100 text-gray-800">
              <MarkdownMessage content={streamBuffer} isStreaming />
              <span className="inline-block w-1.5 h-3 bg-gray-400 ml-0.5 animate-pulse" />
            </div>
          </div>
        )}

        <div ref={bottomRef} />
      </div>

      {/* Input */}
      <div className="border-t border-gray-100 px-6 py-4 flex-shrink-0">
        {errorMessage && (
          <div className="mb-3 flex items-center gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            <AlertCircle size={15} />
            <span>{errorMessage}</span>
          </div>
        )}
        <div className="flex items-end gap-3">
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage() } }}
            placeholder="Type a message… (Enter to send)"
            rows={1}
            disabled={streaming}
            className="flex-1 text-sm px-3 py-2 border border-gray-200 rounded-lg resize-none focus:outline-none focus:ring-1 focus:ring-purple-400 disabled:opacity-50"
            style={{ minHeight: 40, maxHeight: 120 }}
          />
          <button
            onClick={sendMessage}
            disabled={!input.trim() || streaming}
            className="px-3 py-2 bg-purple-600 text-white rounded-lg hover:bg-purple-700 disabled:opacity-50 flex items-center gap-1.5"
          >
            <Send size={15} />
          </button>
        </div>
      </div>
    </div>
  )
}
