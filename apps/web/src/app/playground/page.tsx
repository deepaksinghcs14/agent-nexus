'use client'

import { useState, Suspense } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { useQuery, useMutation } from '@tanstack/react-query'
import { Bot, ArrowRight } from 'lucide-react'
import { agentsAPI, conversationsAPI } from '@/lib/api'
import type { Agent } from '@/types'

function PlaygroundInner() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const preselectedAgent = searchParams.get('agent') ?? ''

  const { data } = useQuery({
    queryKey: ['agents'],
    queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }>,
  })

  const [selectedAgent, setSelectedAgent] = useState(preselectedAgent)
  const [title, setTitle] = useState('')
  const [error, setError] = useState('')

  const agents = data?.data ?? []

  const createMutation = useMutation({
    mutationFn: () => conversationsAPI.create({
      agent_id: selectedAgent,
      title: title || 'New Conversation',
    }),
    onSuccess: (conv: unknown) => {
      const c = conv as { id: string }
      router.push(`/playground/${c.id}`)
    },
    onError: (err: Error) => setError(err.message),
  })

  return (
    <div className="p-6 max-w-xl">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-gray-900">Playground</h1>
        <p className="text-sm text-gray-500 mt-0.5">Start a new conversation with an agent</p>
      </div>

      <div className="border border-gray-100 rounded-xl p-5 bg-white">
        <div className="mb-4">
          <label className="block text-sm font-medium text-gray-700 mb-2">Select Agent</label>
          {agents.length === 0 ? (
            <p className="text-sm text-gray-400">No agents yet. <a href="/agents/new" className="text-purple-600 underline">Create one</a>.</p>
          ) : (
            <div className="space-y-2">
              {agents.map((agent) => (
                <button
                  key={agent.id}
                  onClick={() => setSelectedAgent(agent.id)}
                  className={`w-full text-left p-3 rounded-lg border transition-all ${
                    selectedAgent === agent.id
                      ? 'border-purple-400 bg-purple-50'
                      : 'border-gray-100 hover:border-gray-200 bg-white'
                  }`}
                >
                  <div className="flex items-center gap-3">
                    <div className={`w-8 h-8 rounded-lg flex items-center justify-center ${
                      selectedAgent === agent.id ? 'bg-purple-200' : 'bg-gray-100'
                    }`}>
                      <Bot size={16} className={selectedAgent === agent.id ? 'text-purple-700' : 'text-gray-400'} />
                    </div>
                    <div>
                      <p className="text-sm font-medium text-gray-900">{agent.name}</p>
                      {agent.description && (
                        <p className="text-xs text-gray-400 truncate">{agent.description}</p>
                      )}
                    </div>
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="mb-4">
          <label className="block text-sm font-medium text-gray-700 mb-1">Conversation title (optional)</label>
          <input
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="e.g. Review auth system"
            className="w-full text-sm px-3 py-2 border border-gray-200 rounded-lg focus:outline-none focus:ring-1 focus:ring-purple-400"
          />
        </div>

        {error && (
          <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded px-3 py-2 mb-3">{error}</div>
        )}

        <button
          onClick={() => createMutation.mutate()}
          disabled={!selectedAgent || createMutation.isPending}
          className="w-full flex items-center justify-center gap-2 py-2.5 bg-purple-600 text-white text-sm rounded-lg hover:bg-purple-700 disabled:opacity-50"
        >
          {createMutation.isPending ? 'Starting…' : <>Start Conversation <ArrowRight size={15} /></>}
        </button>
      </div>
    </div>
  )
}

export default function PlaygroundPage() {
  return (
    <Suspense fallback={<div className="p-6 text-sm text-gray-400">Loading…</div>}>
      <PlaygroundInner />
    </Suspense>
  )
}
