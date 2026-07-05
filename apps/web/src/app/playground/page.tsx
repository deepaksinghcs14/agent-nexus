'use client'

import { useState, Suspense } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { useQuery, useMutation } from '@tanstack/react-query'
import { Bot, ArrowRight, Cpu, MessageSquare } from 'lucide-react'
import { agentsAPI, conversationsAPI } from '@/lib/api'
import type { Agent } from '@/types'

function modelShort(model: string) {
  return model.split('/').pop()?.split('-').slice(0, 3).join('-') ?? model
}

function PlaygroundInner() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const preselectedAgent = searchParams.get('agent') ?? ''

  const { data } = useQuery({
    queryKey: ['agents'],
    queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }>,
  })

  const [selectedAgent, setSelectedAgent] = useState(preselectedAgent)
  const [firstMessage, setFirstMessage] = useState('')
  const [error, setError] = useState('')

  const agents = (data?.data ?? []).filter((a: Agent) => a.status !== 'archived')

  const createMutation = useMutation({
    mutationFn: async () => {
      const title = firstMessage.trim()
        ? firstMessage.trim().slice(0, 60)
        : 'New Conversation'
      const conv = await conversationsAPI.create({ agent_id: selectedAgent, title }) as { id: string }
      return { convId: conv.id, message: firstMessage.trim() }
    },
    onSuccess: ({ convId, message }) => {
      if (message) {
        router.push(`/playground/${convId}?msg=${encodeURIComponent(message)}`)
      } else {
        router.push(`/playground/${convId}`)
      }
    },
    onError: (err: Error) => setError(err.message),
  })

  return (
    <div className="p-4 sm:p-6 max-w-2xl">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">Playground</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">Start a conversation with an agent</p>
      </div>

      <div className="space-y-5">
        {/* Agent selection */}
        <div>
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Choose an agent</label>
          {agents.length === 0 ? (
            <div className="rounded-xl border border-dashed border-gray-200 dark:border-gray-700 p-6 text-center">
              <Bot size={24} className="text-gray-300 dark:text-gray-600 mx-auto mb-2" />
              <p className="text-sm text-gray-500 dark:text-gray-400">No agents yet.</p>
              <a href="/agents/new" className="text-sm text-purple-600 dark:text-purple-300 hover:underline font-medium">Create your first agent →</a>
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
              {agents.map((agent: Agent) => (
                <button
                  key={agent.id}
                  onClick={() => setSelectedAgent(agent.id)}
                  className={`text-left p-3 rounded-xl border transition-all ${
                    selectedAgent === agent.id
                      ? 'border-purple-400 bg-purple-50 dark:bg-purple-500/10 ring-1 ring-purple-200'
                      : 'border-gray-100 dark:border-gray-800 hover:border-gray-200 bg-white dark:bg-gray-900 hover:bg-gray-50 dark:hover:bg-gray-800'
                  }`}
                >
                  <div className="flex items-start gap-3">
                    <div className={`w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0 ${
                      selectedAgent === agent.id ? 'bg-purple-200' : 'bg-gray-100 dark:bg-gray-800'
                    }`}>
                      <Bot size={16} className={selectedAgent === agent.id ? 'text-purple-700 dark:text-purple-300' : 'text-gray-400 dark:text-gray-500'} />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">{agent.name}</p>
                      {agent.description && (
                        <p className="text-xs text-gray-400 dark:text-gray-500 truncate mt-0.5">{agent.description}</p>
                      )}
                      <div className="flex items-center gap-1 mt-1.5">
                        <Cpu size={9} className="text-gray-400 dark:text-gray-500" />
                        <span className="text-[10px] text-gray-400 dark:text-gray-500 font-medium">{modelShort(agent.model)}</span>
                      </div>
                    </div>
                    {selectedAgent === agent.id && (
                      <div className="w-4 h-4 rounded-full bg-purple-500 flex items-center justify-center flex-shrink-0 mt-0.5">
                        <div className="w-1.5 h-1.5 rounded-full bg-white dark:bg-gray-900" />
                      </div>
                    )}
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* First message */}
        {selectedAgent && (
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
              <span className="flex items-center gap-1.5">
                <MessageSquare size={14} />
                First message <span className="text-gray-400 dark:text-gray-500 font-normal">(optional)</span>
              </span>
            </label>
            <textarea
              value={firstMessage}
              onChange={e => setFirstMessage(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && selectedAgent) { e.preventDefault(); createMutation.mutate() } }}
              placeholder="What would you like to ask? Skip to start with an empty conversation."
              rows={3}
              className="w-full text-sm px-3 py-2.5 border border-gray-200 dark:border-gray-700 rounded-xl resize-none focus:outline-none focus:ring-1 focus:ring-purple-400 focus:border-purple-300 bg-white dark:bg-gray-900 placeholder-gray-400 dark:placeholder-gray-500"
            />
          </div>
        )}

        {error && (
          <div className="text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg px-3 py-2">{error}</div>
        )}

        <button
          onClick={() => createMutation.mutate()}
          disabled={!selectedAgent || createMutation.isPending}
          className="w-full flex items-center justify-center gap-2 py-2.5 bg-purple-600 text-white text-sm font-medium rounded-xl hover:bg-purple-700 disabled:opacity-50 transition-colors"
        >
          {createMutation.isPending ? 'Starting…' : (
            <>
              {firstMessage.trim() ? 'Send message' : 'Start conversation'}
              <ArrowRight size={15} />
            </>
          )}
        </button>
      </div>
    </div>
  )
}

export default function PlaygroundPage() {
  return (
    <Suspense fallback={<div className="p-6 text-sm text-gray-400 dark:text-gray-500">Loading…</div>}>
      <PlaygroundInner />
    </Suspense>
  )
}
