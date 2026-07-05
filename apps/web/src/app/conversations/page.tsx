'use client'

import Link from 'next/link'
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { MessageSquare, Trash2, Plus } from 'lucide-react'
import { conversationsAPI } from '@/lib/api'
import { relativeTime } from '@/lib/utils'
import type { Conversation } from '@/types'

export default function ConversationsPage() {
  const queryClient = useQueryClient()
  const [error, setError] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['conversations'],
    queryFn: () => conversationsAPI.list() as Promise<{ data: Conversation[] }>,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => conversationsAPI.delete(id),
    onSuccess: () => { setError(''); queryClient.invalidateQueries({ queryKey: ['conversations'] }) },
    onError: (e: Error) => setError(e.message || 'Failed to delete conversation'),
  })

  const deleteAllMutation = useMutation({
    mutationFn: () => conversationsAPI.deleteAll(),
    onSuccess: () => { setError(''); queryClient.invalidateQueries({ queryKey: ['conversations'] }) },
    onError: (e: Error) => setError(e.message || 'Failed to clear conversations'),
  })

  const conversations = data?.data ?? []

  return (
    <div className="p-4 sm:p-6 max-w-4xl">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-6">
        <div>
          <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">Conversations</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">{conversations.length} conversation{conversations.length !== 1 ? 's' : ''}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {conversations.length > 0 && (
            <button
              onClick={() => { if (confirm(`Clear all ${conversations.length} conversations? This cannot be undone.`)) deleteAllMutation.mutate() }}
              disabled={deleteAllMutation.isPending}
              className="flex items-center gap-1.5 px-3 py-1.5 border border-red-200 text-red-600 dark:text-red-300 text-sm rounded-lg hover:bg-red-50 disabled:opacity-50"
            >
              <Trash2 size={14} /> Clear all
            </button>
          )}
          <Link href="/playground">
            <button className="flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-sm rounded-lg hover:bg-purple-700">
              <Plus size={15} /> New Chat
            </button>
          </Link>
        </div>
      </div>

      {error && <div className="mb-4 text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg px-3 py-2">{error}</div>}

      {isLoading && <div className="text-sm text-gray-400 dark:text-gray-500 py-12 text-center">Loading…</div>}

      {!isLoading && conversations.length === 0 && (
        <div className="border border-dashed border-gray-200 dark:border-gray-700 rounded-xl p-12 text-center">
          <MessageSquare size={32} className="mx-auto text-gray-300 dark:text-gray-600 mb-3" />
          <p className="text-gray-500 dark:text-gray-400 text-sm">No conversations yet. Start one from the Playground.</p>
          <Link href="/playground">
            <button className="mt-4 px-4 py-2 bg-purple-600 text-white text-sm rounded-lg hover:bg-purple-700">
              Open Playground
            </button>
          </Link>
        </div>
      )}

      <div className="space-y-2">
        {conversations.map((c) => (
          <div key={c.id} className="flex items-center justify-between border border-gray-100 dark:border-gray-800 rounded-xl p-4 bg-white dark:bg-gray-900 hover:border-gray-200">
            <Link href={`/playground/${c.id}`} className="flex-1 min-w-0">
              <p className="text-sm font-medium text-gray-900 dark:text-gray-100 truncate">{c.title}</p>
              <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">
                {c.message_count} message{c.message_count !== 1 ? 's' : ''} · {relativeTime(c.updated_at)}
              </p>
            </Link>
            <button
              onClick={() => { if (confirm('Delete this conversation?')) deleteMutation.mutate(c.id) }}
              disabled={deleteMutation.isPending}
              className="p-1.5 text-gray-400 dark:text-gray-500 hover:text-red-500 hover:bg-red-50 rounded-lg ml-3 disabled:opacity-40"
            >
              <Trash2 size={14} />
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}
