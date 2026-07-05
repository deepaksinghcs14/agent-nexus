'use client'

import React, { useState, useRef, useEffect, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import Link from 'next/link'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  Sparkles, Send, Loader2, CheckCircle2, AlertCircle,
  ExternalLink, ArrowRight, ChevronDown, Zap,
} from 'lucide-react'
import { providersAPI, nexusAIAPI } from '@/lib/api'
import type { NexusMessage, ProviderCredential, ModelInfo } from '@/types'

const SUGGESTIONS = [
  'Create a research agent with web search that writes detailed reports',
  'Build a supervisor workflow: coordinator delegates to researcher, fact-checker, and writer agents',
  'Build a support triage workflow: classify → route → respond',
  'Make a code review pipeline with a reviewer and a fixer agent',
  'Set up a webhook trigger to run a workflow from GitHub, Zapier, or any HTTP source',
]

const TEMPLATES = [
  {
    icon: 'zap' as const,
    title: 'Webhook trigger setup',
    desc: 'Wire an external HTTP event to a workflow',
    prompt: `I need to set up a webhook trigger for a workflow.

Please:
1. List my existing workflows so I can pick the right one
2. Create a webhook trigger for it

Details:
- Trigger name: [e.g. "GitHub PR Webhook"]
- Trigger source: [e.g. GitHub pull_request event, Zapier, custom HTTP]
- Payload mapping: [e.g. use {{.Body.pull_request.title}} or just pass {{.RawBody}}]
- HMAC secret needed: [yes / no]`,
  },
]

let _counter = 0
const uid = () => `m-${Date.now()}-${_counter++}`

// ── Tool event card ──────────────────────────────────────────────────────────

function ToolCard({ msg }: { msg: NexusMessage }) {
  const ev = msg.toolEvent
  if (!ev) return null
  const started = ev.status === 'started'
  const isError = !!ev.error

  return (
    <div className={`flex items-start gap-3 px-3.5 py-2.5 rounded-lg border text-sm
      ${isError ? 'bg-red-50 dark:bg-red-500/10 border-red-200' : started ? 'bg-purple-50 dark:bg-purple-500/10 border-purple-200' : 'bg-green-50 dark:bg-green-500/10 border-green-200'}`}
    >
      <div className="mt-0.5 shrink-0">
        {isError ? <AlertCircle size={15} className="text-red-500" />
          : started ? <Loader2 size={15} className="animate-spin text-purple-500" />
          : <CheckCircle2 size={15} className="text-green-500" />}
      </div>
      <div className="flex-1 min-w-0">
        <p className={`font-medium text-[13px] ${isError ? 'text-red-700 dark:text-red-300' : started ? 'text-purple-700 dark:text-purple-300' : 'text-green-700 dark:text-green-300'}`}>
          {ev.label}
        </p>
        {ev.error && <p className="text-red-500 text-xs mt-0.5">{ev.error}</p>}
        {ev.result && !started && <p className="text-gray-500 dark:text-gray-400 text-xs mt-0.5">{ev.result.name}</p>}
      </div>
      {ev.link && !started && !isError && (
        <Link href={ev.link} className="flex items-center gap-1 text-xs text-purple-600 dark:text-purple-300 hover:text-purple-700 font-medium shrink-0">
          Open <ExternalLink size={11} />
        </Link>
      )}
    </div>
  )
}

// ── Message bubble ───────────────────────────────────────────────────────────

function MessageBubble({ msg, isStreaming }: { msg: NexusMessage; isStreaming: boolean }) {
  if (msg.role === 'tool_event') {
    return <div className="pl-9"><ToolCard msg={msg} /></div>
  }
  if (msg.role === 'user') {
    return (
      <div className="flex justify-end">
        <div className="max-w-[90%] sm:max-w-[72%] px-4 py-2.5 rounded-2xl rounded-tr-sm bg-purple-600 text-white text-sm leading-relaxed whitespace-pre-wrap">
          {msg.content}
        </div>
      </div>
    )
  }
  return (
    <div className="flex gap-2.5 items-start">
      <div className="mt-0.5 shrink-0 w-6 h-6 rounded-full bg-purple-100 flex items-center justify-center">
        <Sparkles size={12} className="text-purple-600 dark:text-purple-300" />
      </div>
      <div className="flex-1 text-sm text-gray-800 dark:text-gray-200 leading-relaxed min-w-0">
        {isStreaming && msg.content === '' ? (
          <span className="flex items-center gap-1.5 text-gray-400 dark:text-gray-500">
            <Loader2 size={13} className="animate-spin" /><span>thinking…</span>
          </span>
        ) : (
          <>
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={{
                p: ({ children }: { children?: React.ReactNode }) => <p className="mb-2 last:mb-0">{children}</p>,
                a: ({ href, children }: { href?: string; children?: React.ReactNode }) => (
                  <a href={href} target="_blank" rel="noopener noreferrer"
                    className="text-purple-600 dark:text-purple-300 hover:text-purple-700 underline underline-offset-2">
                    {children}
                  </a>
                ),
                ul: ({ children }: { children?: React.ReactNode }) => <ul className="list-disc pl-4 mb-2 space-y-0.5">{children}</ul>,
                ol: ({ children }: { children?: React.ReactNode }) => <ol className="list-decimal pl-4 mb-2 space-y-0.5">{children}</ol>,
                li: ({ children }: { children?: React.ReactNode }) => <li className="text-sm">{children}</li>,
                code: ({ children, className }: { children?: React.ReactNode; className?: string }) => {
                  const isBlock = className?.includes('language-')
                  return isBlock
                    ? <code className="block bg-gray-100 dark:bg-gray-800 rounded px-3 py-2 text-xs font-mono overflow-x-auto mb-2">{children}</code>
                    : <code className="bg-gray-100 dark:bg-gray-800 rounded px-1 py-0.5 text-xs font-mono">{children}</code>
                },
                strong: ({ children }: { children?: React.ReactNode }) => <strong className="font-semibold text-gray-900 dark:text-gray-100">{children}</strong>,
                h1: ({ children }: { children?: React.ReactNode }) => <h1 className="text-base font-semibold text-gray-900 dark:text-gray-100 mb-1 mt-2">{children}</h1>,
                h2: ({ children }: { children?: React.ReactNode }) => <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100 mb-1 mt-2">{children}</h2>,
                h3: ({ children }: { children?: React.ReactNode }) => <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-1 mt-2">{children}</h3>,
              }}
            >
              {msg.content}
            </ReactMarkdown>
            {isStreaming && (
              <span className="inline-block w-0.5 h-3.5 bg-purple-500 animate-pulse ml-0.5 align-middle" />
            )}
          </>
        )}
      </div>
    </div>
  )
}

// ── Main page ────────────────────────────────────────────────────────────────

export default function NexusAIPage() {
  const [messages, setMessages] = useState<NexusMessage[]>([{
    id: uid(), role: 'assistant',
    content: "Hi! I'm Nexus AI. Describe the agent or workflow you'd like to build and I'll create it instantly — with memory, tools, connectors, and multi-step pipelines — all from natural language.",
  }])
  const [input, setInput] = useState('')
  const [isRunning, setIsRunning] = useState(false)
  const [activeAssistantId, setActiveAssistantId] = useState<string | null>(null)

  // Provider/model selection
  const [selectedProvider, setSelectedProvider] = useState<string>('')
  const [selectedModel, setSelectedModel] = useState<string>('')

  const bottomRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  // ── Data fetching ──────────────────────────────────────────────────────────

  const { data: providersData, isLoading: providersLoading } = useQuery({
    queryKey: ['providers'],
    queryFn: () => providersAPI.list() as Promise<{ data: ProviderCredential[] }>,
  })
  const activeProviders = providersData?.data?.filter(p => p.is_active) ?? []
  const hasProvider = activeProviders.length > 0

  // Auto-select first provider when loaded
  useEffect(() => {
    if (activeProviders.length > 0 && !selectedProvider) {
      setSelectedProvider(activeProviders[0].provider)
    }
  }, [activeProviders.length]) // eslint-disable-line react-hooks/exhaustive-deps

  // Find the credential ID for the selected provider (needed to fetch its models)
  const activeCred = activeProviders.find(p => p.provider === selectedProvider)

  const { data: modelsData, isLoading: modelsLoading } = useQuery({
    queryKey: ['provider-models', activeCred?.id],
    queryFn: () => providersAPI.models(activeCred!.id) as Promise<{ data: ModelInfo[] }>,
    enabled: !!activeCred?.id,
  })
  const availableModels = modelsData?.data ?? []

  // Auto-select first model when models load or provider changes
  useEffect(() => {
    if (availableModels.length > 0) {
      setSelectedModel(availableModels[0].id)
    } else {
      setSelectedModel('')
    }
  }, [modelsData, selectedProvider]) // eslint-disable-line react-hooks/exhaustive-deps

  // ── Scroll / textarea resize ───────────────────────────────────────────────

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  useEffect(() => {
    const ta = textareaRef.current
    if (!ta) return
    ta.style.height = 'auto'
    ta.style.height = Math.min(ta.scrollHeight, 160) + 'px'
  }, [input])

  // ── Message helpers ────────────────────────────────────────────────────────

  const appendToAssistant = useCallback((id: string, chunk: string) => {
    setMessages(prev => prev.map(m => m.id === id ? { ...m, content: m.content + chunk } : m))
  }, [])

  const upsertToolCard = useCallback((
    event: NonNullable<NexusMessage['toolEvent']>,
    existingId?: string
  ) => {
    if (existingId) {
      setMessages(prev => prev.map(m => m.id === existingId ? { ...m, toolEvent: event } : m))
      return existingId
    }
    const id = uid()
    setMessages(prev => [...prev, { id, role: 'tool_event', content: '', toolEvent: event }])
    return id
  }, [])

  // ── Send ───────────────────────────────────────────────────────────────────

  const sendMessage = useCallback(async (text: string) => {
    if (!text.trim() || isRunning) return
    setInput('')
    setIsRunning(true)

    const userMsg: NexusMessage = { id: uid(), role: 'user', content: text.trim() }
    const assistantId = uid()
    setMessages(prev => [...prev, userMsg, { id: assistantId, role: 'assistant', content: '' }])
    setActiveAssistantId(assistantId)

    // Only send real conversation turns — exclude tool_event cards and the
    // initial UI-only greeting (assistant message before any user message).
    const allMsgs = [...messages, userMsg].filter(m => m.role === 'user' || m.role === 'assistant')
    const firstUserIdx = allMsgs.findIndex(m => m.role === 'user')
    const history = allMsgs
      .slice(firstUserIdx >= 0 ? firstUserIdx : 0)
      .map(m => ({ role: m.role as 'user' | 'assistant', content: m.content }))

    const toolCardIds = new Map<string, string>()

    try {
      const res = await nexusAIAPI.chat(history, {
        provider: selectedProvider || undefined,
        model: selectedModel || undefined,
      })
      if (!res.ok || !res.body) throw new Error('Connection failed')

      const reader = res.body.getReader()
      const dec = new TextDecoder()
      let buf = ''

      while (true) {
        const { value, done } = await reader.read()
        if (done) break
        buf += dec.decode(value, { stream: true })
        const lines = buf.split('\n')
        buf = lines.pop() ?? ''

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          const raw = line.slice(6).trim()
          if (!raw) continue
          let evt: Record<string, unknown>
          try { evt = JSON.parse(raw) } catch { continue }

          switch (evt.type) {
            case 'delta':
              appendToAssistant(assistantId, (evt.content as string) ?? '')
              break
            case 'tool_started': {
              const tool = (evt.tool as string) ?? ''
              const label = (evt.label as string) ?? tool
              const cardId = upsertToolCard({ status: 'started', tool, label })
              toolCardIds.set(tool + '_pending', cardId)
              break
            }
            case 'tool_completed': {
              const tool = (evt.tool as string) ?? ''
              const label = (evt.label as string) ?? 'Done'
              const existingId = toolCardIds.get(tool + '_pending')
              toolCardIds.delete(tool + '_pending')
              upsertToolCard({
                status: 'completed', tool, label,
                result: evt.result as { id: string; name: string } | undefined,
                link: evt.link as string | undefined,
                error: evt.error as string | undefined,
              }, existingId)
              break
            }
            case 'error':
              setMessages(prev => prev.map(m =>
                m.id === assistantId && !m.content
                  ? { ...m, content: `Sorry, something went wrong: ${(evt.error as string) ?? 'unknown error'}` }
                  : m
              ))
              break
          }
        }
      }
    } catch {
      setMessages(prev => prev.map(m =>
        m.id === assistantId && !m.content
          ? { ...m, content: 'Connection error. Please try again.' }
          : m
      ))
    } finally {
      setIsRunning(false)
      setActiveAssistantId(null)
    }
  }, [messages, isRunning, selectedProvider, selectedModel, appendToAssistant, upsertToolCard])

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendMessage(input) }
  }

  // ── Loading ────────────────────────────────────────────────────────────────

  if (providersLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 size={20} className="animate-spin text-gray-400 dark:text-gray-500" />
      </div>
    )
  }

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <div className="flex flex-col h-full max-w-4xl mx-auto">
      {/* Header */}
      <div className="px-4 sm:px-6 py-3.5 border-b border-gray-100 dark:border-gray-800 flex flex-wrap items-center gap-3 flex-shrink-0">
        <div className="flex items-center gap-2 flex-shrink-0">
          <div className="w-7 h-7 rounded-lg bg-purple-100 flex items-center justify-center">
            <Sparkles size={14} className="text-purple-600 dark:text-purple-300" />
          </div>
          <span className="text-[15px] font-semibold text-gray-900 dark:text-gray-100">Nexus AI</span>
        </div>

        {/* Provider + model selectors */}
        {hasProvider && (
          <div className="flex flex-wrap items-center gap-2 ml-auto">
            {/* Provider selector */}
            <div className="relative">
              <select
                value={selectedProvider}
                onChange={e => {
                  setSelectedProvider(e.target.value)
                  setSelectedModel('')
                }}
                disabled={isRunning}
                className="appearance-none pl-3 pr-7 py-1.5 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 text-sm text-gray-700 dark:text-gray-300 focus:outline-none focus:border-purple-300 focus:ring-2 focus:ring-purple-100 disabled:opacity-50 cursor-pointer"
              >
                {activeProviders.map(p => (
                  <option key={p.id} value={p.provider}>
                    {p.display_name || p.provider}
                  </option>
                ))}
              </select>
              <ChevronDown size={13} className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500 pointer-events-none" />
            </div>

            {/* Model selector */}
            <div className="relative">
              <select
                value={selectedModel}
                onChange={e => setSelectedModel(e.target.value)}
                disabled={isRunning || modelsLoading || availableModels.length === 0}
                className="appearance-none pl-3 pr-7 py-1.5 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 text-sm text-gray-700 dark:text-gray-300 focus:outline-none focus:border-purple-300 focus:ring-2 focus:ring-purple-100 disabled:opacity-50 cursor-pointer min-w-0 w-full sm:min-w-[160px] sm:w-auto"
              >
                {modelsLoading && <option value="">Loading models…</option>}
                {!modelsLoading && availableModels.length === 0 && (
                  <option value="">No models available</option>
                )}
                {availableModels.map(m => (
                  <option key={m.id} value={m.id}>{m.name || m.id}</option>
                ))}
              </select>
              <ChevronDown size={13} className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500 pointer-events-none" />
            </div>
          </div>
        )}
      </div>

      {/* No-provider banner */}
      {!hasProvider && (
        <div className="mx-4 sm:mx-6 mt-4 flex items-center gap-3 px-4 py-3 rounded-lg bg-amber-50 dark:bg-amber-500/10 border border-amber-200 flex-shrink-0">
          <AlertCircle size={15} className="text-amber-500 shrink-0" />
          <p className="text-sm text-amber-800 dark:text-amber-300">
            Nexus AI uses your own LLM provider. Add one in{' '}
            <Link href="/settings/providers" className="font-medium underline hover:text-amber-900">
              Settings → Providers
            </Link>{' '}
            to get started.
          </p>
        </div>
      )}

      {/* Messages */}
      <div className="flex-1 overflow-y-auto px-4 sm:px-6 py-5 space-y-4">
        {messages.length === 0 && (
          <div className="flex-1 flex flex-col items-center justify-center px-6 py-12">
            <div className="w-10 h-10 rounded-full bg-purple-100 flex items-center justify-center mb-4">
              <Sparkles size={20} className="text-purple-600 dark:text-purple-300" />
            </div>
            <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-1">Nexus AI</h2>
            <p className="text-sm text-gray-500 dark:text-gray-400 mb-8 text-center">Ask me to build agents, create workflows, or manage your workspace</p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 w-full max-w-lg">
              {[
                { label: 'Create a research agent', desc: 'With web search and report generation' },
                { label: 'Build a support workflow', desc: 'Classify → route → respond pipeline' },
                { label: 'List my agents', desc: 'See all agents in this workspace' },
                { label: 'Create a webhook trigger', desc: 'Connect an external event to a workflow' },
              ].map((item) => (
                <button
                  key={item.label}
                  onClick={() => setInput(item.label)}
                  className="text-left p-3.5 rounded-xl border border-gray-100 dark:border-gray-800 hover:border-purple-200 hover:bg-purple-50/30 transition-all"
                >
                  <p className="text-sm font-medium text-gray-800 dark:text-gray-200">{item.label}</p>
                  <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">{item.desc}</p>
                </button>
              ))}
            </div>
          </div>
        )}
        {messages.map(msg => (
          <MessageBubble
            key={msg.id}
            msg={msg}
            isStreaming={isRunning && msg.id === activeAssistantId}
          />
        ))}
        <div ref={bottomRef} />
      </div>

      {/* Template cards + suggestion chips — only on first greeting */}
      {messages.length === 1 && hasProvider && (
        <div className="px-4 sm:px-6 pb-3 flex-shrink-0 space-y-3">
          {/* Template cards */}
          <div className="flex gap-2">
            {TEMPLATES.map(t => (
              <button
                key={t.title}
                onClick={() => setInput(t.prompt)}
                className="flex items-start gap-2.5 px-3.5 py-2.5 rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 text-left hover:border-purple-300 hover:bg-purple-50 transition-colors flex-1 min-w-0"
              >
                <div className="w-6 h-6 rounded-md bg-purple-100 flex items-center justify-center flex-shrink-0 mt-0.5">
                  <Zap size={12} className="text-purple-600 dark:text-purple-300" />
                </div>
                <div className="min-w-0">
                  <p className="text-[12px] font-semibold text-gray-800 dark:text-gray-200 truncate">{t.title}</p>
                  <p className="text-[11px] text-gray-400 dark:text-gray-500 truncate">{t.desc}</p>
                </div>
              </button>
            ))}
          </div>
          {/* Suggestion chips */}
          <div className="flex flex-wrap gap-2">
            {SUGGESTIONS.map(s => (
              <button
                key={s}
                onClick={() => setInput(s)}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 text-xs text-gray-600 dark:text-gray-400 hover:border-purple-300 hover:text-purple-700 hover:bg-purple-50 transition-colors"
              >
                <ArrowRight size={11} className="text-gray-400 dark:text-gray-500" />
                {s.length > 60 ? s.slice(0, 60) + '…' : s}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Input bar */}
      <div className="px-4 sm:px-6 pb-5 pt-1 flex-shrink-0">
        <div className={`flex items-end gap-2.5 border rounded-xl bg-white dark:bg-gray-900 transition-colors
          ${isRunning ? 'border-gray-200 dark:border-gray-700' : 'border-gray-200 dark:border-gray-700 focus-within:border-purple-300 focus-within:ring-2 focus-within:ring-purple-100'}`}
        >
          <textarea
            ref={textareaRef}
            rows={1}
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={
              !hasProvider
                ? 'Add a provider in Settings → Providers to use Nexus AI'
                : 'Describe the agent or workflow you want…  (Enter to send, Shift+Enter for newline)'
            }
            disabled={!hasProvider || isRunning}
            className="flex-1 px-4 py-3 bg-transparent resize-none outline-none text-sm text-gray-800 dark:text-gray-200 placeholder-gray-400 dark:placeholder-gray-500 disabled:opacity-50 min-h-[44px] max-h-[160px] leading-relaxed"
          />
          <div className="pb-2 pr-2">
            <button
              onClick={() => sendMessage(input)}
              disabled={!hasProvider || !input.trim() || isRunning}
              className="w-8 h-8 rounded-lg bg-purple-600 hover:bg-purple-700 disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center transition-colors"
            >
              {isRunning
                ? <Loader2 size={14} className="animate-spin text-white" />
                : <Send size={14} className="text-white" />}
            </button>
          </div>
        </div>
        <p className="text-center text-gray-400 dark:text-gray-500 text-[11px] mt-2">
          Nexus AI creates agents and workflows directly in your workspace. Review before running in production.
        </p>
      </div>
    </div>
  )
}
