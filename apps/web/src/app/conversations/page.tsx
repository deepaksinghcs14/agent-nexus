'use client'

import Link from 'next/link'
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { MessageSquare, Trash2, Plus } from 'lucide-react'
import { conversationsAPI } from '@/lib/api'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable, Row, Cell } from '@/components/ui/DataTable'
import { relativeTime } from '@/lib/utils'
import { useRouter } from 'next/navigation'
import type { Conversation } from '@/types'

export default function ConversationsPage() {
  const queryClient = useQueryClient()
  const router = useRouter()
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
      <PageHeader
        eyebrow="Chat"
        title="Conversations"
        subtitle={`${conversations.length} conversation${conversations.length !== 1 ? 's' : ''} across all agents`}
        actions={
          <>
            {conversations.length > 0 && (
              <button
                onClick={() => { if (confirm(`Clear all ${conversations.length} conversations? This cannot be undone.`)) deleteAllMutation.mutate() }}
                disabled={deleteAllMutation.isPending}
                className="inline-flex items-center gap-1.5 px-3 py-2 border border-crit/30 text-crit text-[13px] rounded-[10px] hover:bg-crit/10 disabled:opacity-50"
              >
                <Trash2 size={14} /> Clear all
              </button>
            )}
            <Link href="/playground" className="inline-flex items-center gap-1.5 px-3.5 py-2 bg-gradient-to-br from-accent to-accent-ink text-white text-[13px] font-semibold rounded-[10px] shadow-card hover:opacity-95">
              <Plus size={15} /> New chat
            </Link>
          </>
        }
      />

      {error && <div className="mb-4 text-sm text-crit bg-crit/10 border border-crit/30 rounded-lg px-3 py-2">{error}</div>}

      {isLoading && <div className="text-sm text-faint py-12 text-center">Loading…</div>}

      {!isLoading && conversations.length === 0 && (
        <div className="border border-dashed border-border-strong rounded-xl p-12 text-center">
          <MessageSquare size={32} className="mx-auto text-faint mb-3" />
          <p className="text-muted-foreground text-sm">No conversations yet. Start one from the Playground.</p>
          <Link href="/playground">
            <button className="mt-4 px-4 py-2 bg-accent text-white text-sm rounded-lg hover:bg-accent-hover">
              Open Playground
            </button>
          </Link>
        </div>
      )}

      {!isLoading && conversations.length > 0 && (
        <DataTable columns={['Conversation', 'Messages', 'Updated', '']} minWidth={560}>
          {conversations.map((c) => (
            <Row key={c.id} onClick={() => router.push(`/playground/${c.id}`)}>
              <Cell className="!whitespace-normal"><span className="font-medium text-foreground">{c.title}</span></Cell>
              <Cell><span className="font-mono text-[11px] text-muted-foreground tabular-nums">{c.message_count}</span></Cell>
              <Cell><span className="text-[11px] text-faint">{relativeTime(c.updated_at)}</span></Cell>
              <Cell className="text-right">
                <button
                  onClick={(e) => { e.stopPropagation(); if (confirm('Delete this conversation?')) deleteMutation.mutate(c.id) }}
                  disabled={deleteMutation.isPending}
                  className="p-1.5 text-faint hover:text-crit hover:bg-crit/10 rounded-lg disabled:opacity-40"
                >
                  <Trash2 size={14} />
                </button>
              </Cell>
            </Row>
          ))}
        </DataTable>
      )}
    </div>
  )
}
