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
import { PageHeader } from '@/components/ui/PageHeader'
import type { NexusMessage, ProviderCredential, ModelInfo } from '@/types'

const TEMPLATES = [
  {
    title: 'Support agent',
    desc: 'Triage + reply drafting',
    prompt: 'Build a support agent that triages my inbound support messages, classifies urgency, tags the topic, and drafts a friendly reply I can approve.',
  },
  {
    title: 'Research pipeline',
    desc: 'Multi-agent + digest',
    prompt: 'Build a daily competitive-intel pipeline: a Market Researcher agent (web search + summarize) feeding a Digest Writer, wired into a scheduled workflow that emails the digest.',
  },
  {
    title: 'Code reviewer',
    desc: 'PR review on webhook',
    prompt: 'Set up a code review pipeline with a reviewer and a fixer agent, triggered by a GitHub pull_request webhook.',
  },
  {
    title: 'Webhook trigger',
    desc: 'Wire an HTTP event to a workflow',
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
      ${isError ? 'bg-crit/10 border-crit/30' : started ? 'bg-accent/10 border-accent/40' : 'bg-good/10 border-good/30'}`}
    >
      <div className="mt-0.5 shrink-0">
        {isError ? <AlertCircle size={15} className="text-crit" />
          : started ? <Loader2 size={15} className="animate-spin text-accent dark:text-accent-bright" />
          : <CheckCircle2 size={15} className="text-good" />}
      </div>
      <div className="flex-1 min-w-0">
        <p className={`font-medium text-[13px] ${isError ? 'text-crit dark:text-crit' : started ? 'text-accent dark:text-accent-bright' : 'text-good'}`}>
          {ev.label}
        </p>
        {ev.error && <p className="text-crit text-xs mt-0.5">{ev.error}</p>}
        {ev.result && !started && <p className="text-muted-foreground text-xs mt-0.5">{ev.result.name}</p>}
      </div>
      {ev.link && !started && !isError && (
        <Link href={ev.link} className="flex items-center gap-1 text-xs text-accent dark:text-accent-bright hover:text-accent dark:text-accent-bright font-medium shrink-0">
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
        <div className="max-w-[90%] sm:max-w-[72%] px-4 py-2.5 rounded-2xl rounded-tr-sm bg-accent text-white text-sm leading-relaxed whitespace-pre-wrap">
          {msg.content}
        </div>
      </div>
    )
  }
  return (
    <div className="flex gap-2.5 items-start">
      <div className="mt-0.5 shrink-0 w-6 h-6 rounded-full bg-accent/15 flex items-center justify-center">
        <Sparkles size={12} className="text-accent dark:text-accent-bright" />
      </div>
      <div className="flex-1 text-sm text-foreground leading-relaxed min-w-0">
        {isStreaming && msg.content === '' ? (
          <span className="flex items-center gap-1.5 text-faint">
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
                    className="text-accent dark:text-accent-bright hover:text-accent dark:text-accent-bright underline underline-offset-2">
                    {children}
                  </a>
                ),
                ul: ({ children }: { children?: React.ReactNode }) => <ul className="list-disc pl-4 mb-2 space-y-0.5">{children}</ul>,
                ol: ({ children }: { children?: React.ReactNode }) => <ol className="list-decimal pl-4 mb-2 space-y-0.5">{children}</ol>,
                li: ({ children }: { children?: React.ReactNode }) => <li className="text-sm">{children}</li>,
                code: ({ children, className }: { children?: React.ReactNode; className?: string }) => {
                  const isBlock = className?.includes('language-')
                  return isBlock
                    ? <code className="block bg-muted rounded px-3 py-2 text-xs font-mono overflow-x-auto mb-2">{children}</code>
                    : <code className="bg-muted rounded px-1 py-0.5 text-xs font-mono">{children}</code>
                },
                strong: ({ children }: { children?: React.ReactNode }) => <strong className="font-semibold text-foreground">{children}</strong>,
                h1: ({ children }: { children?: React.ReactNode }) => <h1 className="text-base font-semibold text-foreground mb-1 mt-2">{children}</h1>,
                h2: ({ children }: { children?: React.ReactNode }) => <h2 className="text-sm font-semibold text-foreground mb-1 mt-2">{children}</h2>,
                h3: ({ children }: { children?: React.ReactNode }) => <h3 className="text-sm font-medium text-foreground mb-1 mt-2">{children}</h3>,
              }}
            >
              {msg.content}
            </ReactMarkdown>
            {isStreaming && (
              <span className="inline-block w-0.5 h-3.5 bg-accent animate-pulse ml-0.5 align-middle" />
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
        <Loader2 size={20} className="animate-spin text-faint" />
      </div>
    )
  }

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <div className="p-4 sm:p-6 max-w-6xl mx-auto h-full flex flex-col">
      <PageHeader
        eyebrow="Build · Natural language"
        title="Nexus AI"
        subtitle="Describe a system in plain language — Nexus discovers your tools, drafts agents, and wires workflows."
      />

      {!hasProvider && (
        <div className="mb-4 flex items-center gap-3 px-4 py-3 rounded-xl bg-warn/10 border border-warn/30 flex-shrink-0">
          <AlertCircle size={15} className="text-warn shrink-0" />
          <p className="text-sm text-warn">
            Nexus AI uses your own LLM provider. Add one in{' '}
            <Link href="/settings/providers" className="font-medium underline hover:text-warn">Settings → Providers</Link>{' '}
            to get started.
          </p>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-[1fr_290px] gap-5 flex-1 min-h-0">
        {/* Chat card */}
        <section className="flex flex-col border border-border rounded-xl bg-surface shadow-card overflow-hidden min-h-0">
          <div className="flex-1 overflow-y-auto px-4 py-5 space-y-4">
            {messages.length === 0 && (
              <div className="h-full flex flex-col items-center justify-center px-6 py-12">
                <div className="w-10 h-10 rounded-full bg-accent/15 flex items-center justify-center mb-4">
                  <Sparkles size={20} className="text-accent dark:text-accent-bright" />
                </div>
                <h2 className="text-lg font-semibold text-foreground mb-1">Nexus AI</h2>
                <p className="text-sm text-muted-foreground text-center">Ask me to build agents, create workflows, or manage your workspace.</p>
              </div>
            )}
            {messages.map(msg => (
              <MessageBubble key={msg.id} msg={msg} isStreaming={isRunning && msg.id === activeAssistantId} />
            ))}
            <div ref={bottomRef} />
          </div>

          {/* Composer */}
          <div className="border-t border-border p-3 flex-shrink-0">
            <div className={`flex items-end gap-2.5 border rounded-xl bg-surface-2 transition-colors
              ${isRunning ? 'border-border-strong' : 'border-border-strong focus-within:border-accent/50 focus-within:ring-2 focus-within:ring-accent/20'}`}>
              <textarea
                ref={textareaRef}
                rows={1}
                value={input}
                onChange={e => setInput(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder={!hasProvider ? 'Add a provider in Settings → Providers to use Nexus AI' : 'Describe the agent or workflow you want…'}
                disabled={!hasProvider || isRunning}
                className="flex-1 px-4 py-3 bg-transparent resize-none outline-none text-sm text-foreground placeholder-faint disabled:opacity-50 min-h-[44px] max-h-[160px] leading-relaxed"
              />
              <div className="pb-2 pr-2">
                <button
                  onClick={() => sendMessage(input)}
                  disabled={!hasProvider || !input.trim() || isRunning}
                  className="w-8 h-8 rounded-lg bg-gradient-to-br from-accent to-accent-ink hover:opacity-95 disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center transition-colors"
                >
                  {isRunning ? <Loader2 size={14} className="animate-spin text-white" /> : <Send size={14} className="text-white" />}
                </button>
              </div>
            </div>
          </div>
        </section>

        {/* Right rail: provider + templates */}
        <aside className="flex flex-col gap-4 min-w-0">
          {hasProvider && (
            <div className="border border-border rounded-xl bg-surface shadow-card overflow-hidden">
              <div className="px-4 py-3 border-b border-border"><h3 className="text-sm font-semibold text-foreground">Provider</h3></div>
              <div className="p-3 flex flex-col gap-2">
                <div className="relative">
                  <select
                    value={selectedProvider}
                    onChange={e => { setSelectedProvider(e.target.value); setSelectedModel('') }}
                    disabled={isRunning}
                    className="w-full appearance-none pl-3 pr-8 py-2 rounded-lg border border-border-strong bg-surface-2 text-[13px] text-foreground focus:outline-none focus:border-accent/50 disabled:opacity-50 cursor-pointer"
                  >
                    {activeProviders.map(p => <option key={p.id} value={p.provider}>{p.display_name || p.provider}</option>)}
                  </select>
                  <ChevronDown size={13} className="absolute right-2.5 top-1/2 -translate-y-1/2 text-faint pointer-events-none" />
                </div>
                <div className="relative">
                  <select
                    value={selectedModel}
                    onChange={e => setSelectedModel(e.target.value)}
                    disabled={isRunning || modelsLoading || availableModels.length === 0}
                    className="w-full appearance-none pl-3 pr-8 py-2 rounded-lg border border-border-strong bg-surface-2 text-[13px] text-foreground focus:outline-none focus:border-accent/50 disabled:opacity-50 cursor-pointer"
                  >
                    {modelsLoading && <option value="">Loading models…</option>}
                    {!modelsLoading && availableModels.length === 0 && <option value="">No models available</option>}
                    {availableModels.map(m => <option key={m.id} value={m.id}>{m.name || m.id}</option>)}
                  </select>
                  <ChevronDown size={13} className="absolute right-2.5 top-1/2 -translate-y-1/2 text-faint pointer-events-none" />
                </div>
              </div>
            </div>
          )}

          <div className="border border-border rounded-xl bg-surface shadow-card overflow-hidden">
            <div className="px-4 py-3 border-b border-border"><h3 className="text-sm font-semibold text-foreground">Start from a template</h3></div>
            <div className="p-2 flex flex-col">
              {TEMPLATES.map(t => (
                <button
                  key={t.title}
                  onClick={() => setInput(t.prompt)}
                  className="flex items-center gap-2.5 px-2.5 py-2.5 rounded-lg text-left hover:bg-muted transition-colors"
                >
                  <div className="w-7 h-7 rounded-md bg-accent/15 flex items-center justify-center flex-shrink-0">
                    <Zap size={13} className="text-accent dark:text-accent-bright" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="text-[12.5px] font-semibold text-foreground truncate">{t.title}</p>
                    <p className="text-[11px] text-faint truncate">{t.desc}</p>
                  </div>
                  <ArrowRight size={13} className="text-faint flex-shrink-0" />
                </button>
              ))}
            </div>
          </div>
        </aside>
      </div>
    </div>
  )
}
