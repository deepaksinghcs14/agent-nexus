'use client'

import Link from 'next/link'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, MessageSquare, Pencil, Trash2, Bot } from 'lucide-react'
import { agentsAPI } from '@/lib/api'
import type { Agent } from '@/types'

const statusColors: Record<string, string> = {
  active: 'bg-green-50 text-green-700',
  paused: 'bg-amber-50 text-amber-700',
  archived: 'bg-gray-50 text-gray-500',
}

const providerColors: Record<string, string> = {
  anthropic: 'bg-amber-50 text-amber-800',
  openai: 'bg-green-50 text-green-800',
  gemini: 'bg-blue-50 text-blue-800',
  ollama: 'bg-gray-50 text-gray-700',
}

export default function AgentsPage() {
  const queryClient = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['agents'],
    queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }>,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => agentsAPI.delete(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  })

  const agents = data?.data ?? []

  return (
    <div className="p-6 max-w-6xl">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Agents</h1>
          <p className="text-sm text-gray-500 mt-0.5">{agents.length} agent{agents.length !== 1 ? 's' : ''}</p>
        </div>
        <Link href="/agents/new">
          <button className="flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-sm rounded-lg hover:bg-purple-700">
            <Plus size={15} /> New Agent
          </button>
        </Link>
      </div>

      {isLoading && (
        <div className="text-sm text-gray-400 py-12 text-center">Loading agents…</div>
      )}

      {!isLoading && agents.length === 0 && (
        <div className="border border-dashed border-gray-200 rounded-xl p-12 text-center">
          <Bot size={32} className="mx-auto text-gray-300 mb-3" />
          <p className="text-gray-500 text-sm">No agents yet.</p>
          <Link href="/agents/new">
            <button className="mt-4 px-4 py-2 bg-purple-600 text-white text-sm rounded-lg hover:bg-purple-700">
              Create your first agent
            </button>
          </Link>
        </div>
      )}

      <div className="grid gap-3">
        {agents.map((agent) => (
          <div key={agent.id} className="border border-gray-100 rounded-xl p-4 bg-white hover:border-gray-200">
            <div className="flex items-start justify-between">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <span className="font-medium text-gray-900 text-sm">{agent.name}</span>
                  <span className={`text-[11px] px-2 py-0.5 rounded-full font-medium ${statusColors[agent.status] ?? statusColors.active}`}>
                    {agent.status}
                  </span>
                  <span className={`text-[11px] px-2 py-0.5 rounded-full font-medium ${providerColors[agent.provider] ?? 'bg-gray-50 text-gray-600'}`}>
                    {agent.provider}
                  </span>
                </div>
                {agent.description && (
                  <p className="text-xs text-gray-500 truncate">{agent.description}</p>
                )}
                <p className="text-xs text-gray-400 mt-1">{agent.model}</p>
              </div>

              <div className="flex items-center gap-1 ml-4">
                <Link href={`/playground?agent=${agent.id}`}>
                  <button className="p-1.5 text-gray-400 hover:text-purple-600 hover:bg-purple-50 rounded-lg" title="Chat">
                    <MessageSquare size={15} />
                  </button>
                </Link>
                <Link href={`/agents/${agent.id}/edit`}>
                  <button className="p-1.5 text-gray-400 hover:text-gray-600 hover:bg-gray-50 rounded-lg" title="Edit">
                    <Pencil size={15} />
                  </button>
                </Link>
                <button
                  onClick={() => { if (confirm('Delete this agent?')) deleteMutation.mutate(agent.id) }}
                  className="p-1.5 text-gray-400 hover:text-red-500 hover:bg-red-50 rounded-lg"
                  title="Delete"
                >
                  <Trash2 size={15} />
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
