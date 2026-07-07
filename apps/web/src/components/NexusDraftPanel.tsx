'use client'

import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Sparkles, Send, Loader2 } from 'lucide-react'
import { nexusAIAPI, providersAPI } from '@/lib/api'
import type { ProviderCredential, ModelInfo } from '@/types'

export type AgentDraft = {
  name?: string
  description?: string
  instructions?: string
  provider?: string
  model?: string
  temperature?: number
  max_tokens?: number
  max_steps?: number
  max_tool_calls?: number
  max_duration_secs?: number
  memory_enabled?: boolean
  memory_scope?: string
  context_retrieval_enabled?: boolean
  agentic_rag?: boolean
  max_chunks?: number
  min_score?: number
  status?: string
  lazy_tool_loading?: boolean
  tools?: { id: string; name: string; description?: string }[]
  skills?: { id: string; name: string }[]
  connectors?: { id: string; name: string }[]
  rationale?: Record<string, string> | null
  unresolved?: string[]
}

type ChatMsg = { role: 'user' | 'assistant'; content: string }

/**
 * NexusDraftPanel — a compact chat box that asks Nexus AI to draft an agent from
 * a plain-language description. On an agent_draft event it calls onDraft() so the
 * parent form pre-fills for review. Nothing is persisted here.
 */
export function NexusDraftPanel({
  onDraft,
}: {
  onDraft: (draft: AgentDraft) => void
}) {
  const [messages, setMessages] = useState<ChatMsg[]>([])
  const [input, setInput] = useState('')
  const [busy, setBusy] = useState(false)
  const [activity, setActivity] = useState('')
  const [drafted, setDrafted] = useState(false)
  const [error, setError] = useState('')
  const streamingRef = useRef('')

  // Provider/model selection — the workspace may not have Anthropic active, so
  // default to the first active provider rather than assuming one.
  const [provider, setProvider] = useState('')
  const [model, setModel] = useState('')
  const { data: providersData } = useQuery({
    queryKey: ['providers'],
    queryFn: () => providersAPI.list() as Promise<{ data: ProviderCredential[] }>,
  })
  const activeProviders = providersData?.data?.filter((p) => p.is_active) ?? []
  const activeCred = activeProviders.find((p) => p.provider === provider)
  const { data: modelsData } = useQuery({
    queryKey: ['provider-models', activeCred?.id],
    queryFn: () => providersAPI.models(activeCred!.id) as Promise<{ data: ModelInfo[] }>,
    enabled: !!activeCred?.id,
  })
  const availableModels = modelsData?.data ?? []

  useEffect(() => {
    if (activeProviders.length > 0 && !provider) setProvider(activeProviders[0].provider)
  }, [activeProviders.length]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    setModel(availableModels.length > 0 ? availableModels[0].id : '')
  }, [modelsData, provider]) // eslint-disable-line react-hooks/exhaustive-deps

  const send = async () => {
    const text = input.trim()
    if (!text || busy) return
    setError('')
    setInput('')
    const history: ChatMsg[] = [...messages, { role: 'user', content: text }]
    setMessages([...history, { role: 'assistant', content: '' }])
    setBusy(true)
    setActivity('Nexus is thinking…')
    streamingRef.current = ''

    try {
      const res = await nexusAIAPI.chat(history, { mode: 'agent_builder', provider: provider || undefined, model: model || undefined })
      if (!res.ok || !res.body) throw new Error(`Nexus request failed (${res.status})`)
      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() ?? ''
        for (const line of lines) {
          const trimmed = line.trim()
          if (!trimmed.startsWith('data:')) continue
          const payload = trimmed.slice(5).trim()
          if (!payload) continue
          let evt: any
          try { evt = JSON.parse(payload) } catch { continue }
          switch (evt.type) {
            case 'delta':
              streamingRef.current += evt.content ?? ''
              setMessages((prev) => {
                const next = [...prev]
                next[next.length - 1] = { role: 'assistant', content: streamingRef.current }
                return next
              })
              break
            case 'tool_started':
              setActivity(evt.label ?? 'Working…')
              break
            case 'tool_completed':
              setActivity(evt.label ?? '')
              break
            case 'agent_draft':
              if (evt.draft) {
                onDraft(evt.draft as AgentDraft)
                setDrafted(true)
              }
              break
            case 'error':
              setError(evt.error ?? 'Nexus error')
              break
          }
        }
      }
    } catch (e: any) {
      setError(e?.message ?? 'Failed to reach Nexus')
    } finally {
      setBusy(false)
      setActivity('')
      // Drop the trailing empty assistant bubble if nothing streamed.
      setMessages((prev) => prev.filter((m, i) => !(i === prev.length - 1 && m.role === 'assistant' && m.content === '')))
    }
  }

  return (
    <div className="mb-6 rounded-xl border border-purple-200 dark:border-purple-500/30 bg-purple-50/40 dark:bg-purple-500/5 overflow-hidden">
      <div className="flex flex-wrap items-center gap-2 px-4 py-2.5 border-b border-purple-100 dark:border-purple-500/20">
        <Sparkles className="w-4 h-4 text-purple-600 dark:text-purple-400" />
        <span className="text-[13px] font-medium text-purple-900 dark:text-purple-200">Describe your agent</span>
        <span className="text-[11px] text-purple-500 dark:text-purple-400/70 mr-auto">Nexus drafts it — you review below</span>
        {activeProviders.length === 0 ? (
          <span className="text-[11px] text-amber-600 dark:text-amber-400">Add a provider in Settings → Providers</span>
        ) : (
          <>
            <select
              value={provider}
              onChange={(e) => { setProvider(e.target.value); setModel('') }}
              className="text-[11px] px-2 py-1 border border-purple-200 dark:border-purple-500/30 rounded-md bg-white dark:bg-gray-900"
            >
              {activeProviders.map((p) => <option key={p.id} value={p.provider}>{p.provider}</option>)}
            </select>
            <select
              value={model}
              onChange={(e) => setModel(e.target.value)}
              disabled={availableModels.length === 0}
              className="text-[11px] px-2 py-1 border border-purple-200 dark:border-purple-500/30 rounded-md bg-white dark:bg-gray-900 max-w-[160px]"
            >
              {availableModels.length === 0 && <option value="">default</option>}
              {availableModels.map((m) => <option key={m.id} value={m.id}>{m.id}</option>)}
            </select>
          </>
        )}
      </div>

      {messages.length > 0 && (
        <div className="px-4 py-3 space-y-2 max-h-56 overflow-y-auto">
          {messages.map((m, i) => (
            <div key={i} className={`text-[13px] leading-relaxed ${m.role === 'user' ? 'text-gray-700 dark:text-gray-300' : 'text-gray-600 dark:text-gray-400'}`}>
              <span className="font-medium">{m.role === 'user' ? 'You: ' : 'Nexus: '}</span>
              <span className="whitespace-pre-wrap">{m.content}</span>
            </div>
          ))}
        </div>
      )}

      {drafted && (
        <div className="mx-4 mb-2 text-[12px] text-green-700 dark:text-green-300 bg-green-50 dark:bg-green-500/10 border border-green-200 dark:border-green-500/20 rounded-lg px-3 py-2">
          ✓ Draft applied to the form below — review, tweak anything, then click Create.
        </div>
      )}
      {error && (
        <div className="mx-4 mb-2 text-[12px] text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg px-3 py-2">{error}</div>
      )}

      <div className="flex items-end gap-2 px-4 py-3">
        <textarea
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() } }}
          disabled={busy}
          rows={2}
          placeholder="e.g. An agent that triages my support inbox, tags urgency, and drafts replies in a friendly tone."
          className="flex-1 text-[13px] px-3 py-2 border border-purple-200 dark:border-purple-500/30 rounded-lg bg-white dark:bg-gray-900 resize-none outline-none focus:ring-1 focus:ring-purple-400 disabled:opacity-60"
        />
        <button
          onClick={send}
          disabled={busy || !input.trim()}
          className="inline-flex items-center gap-1.5 px-3 py-2 bg-purple-600 text-white text-[12px] rounded-lg hover:bg-purple-700 disabled:opacity-50 shrink-0"
        >
          {busy ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Send className="w-3.5 h-3.5" />}
          {drafted ? 'Refine' : 'Draft'}
        </button>
      </div>
      {activity && <div className="px-4 pb-3 -mt-1 text-[11px] text-purple-500 dark:text-purple-400 flex items-center gap-1.5"><Loader2 className="w-3 h-3 animate-spin" />{activity}</div>}
    </div>
  )
}
