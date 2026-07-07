'use client'

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Building2, Bot, Activity, Coins, Trash2 } from 'lucide-react'
import { adminAPI } from '@/lib/api'
import { relativeTime, formatTokens } from '@/lib/utils'
import type { Workspace } from '@/types'

interface WorkspaceWithStats extends Workspace {
  agent_count: number
  run_count: number
  total_tokens: number
}

export default function AdminWorkspacesPage() {
  const qc = useQueryClient()
  const [confirmDelete, setConfirmDelete] = useState<WorkspaceWithStats | null>(null)
  const [deleteError, setDeleteError] = useState('')

  const { data, isLoading, error } = useQuery({
    queryKey: ['admin-workspaces'],
    queryFn: () => adminAPI.workspaces() as Promise<{ data: WorkspaceWithStats[] }>,
  })
  const workspaces = data?.data ?? []

  const deleteMut = useMutation({
    mutationFn: (id: string) => adminAPI.deleteWorkspace(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-workspaces'] })
      qc.invalidateQueries({ queryKey: ['admin-usage'] })
      setConfirmDelete(null)
      setDeleteError('')
    },
    onError: (e: Error) => setDeleteError(e.message),
  })

  return (
    <div className="p-4 sm:p-6 max-w-5xl">
      <div className="mb-5">
        <h1 className="text-xl font-semibold text-foreground">Workspaces</h1>
        <p className="text-sm text-muted-foreground mt-0.5">Manage all workspaces on this instance</p>
      </div>
      {error && (
        <div className="text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg p-3 mb-4">{(error as Error).message}</div>
      )}
      {isLoading && <div className="py-12 text-center text-sm text-faint">Loading…</div>}
      {!isLoading && !error && workspaces.length === 0 && (
        <div className="border border-dashed border-border-strong rounded-xl py-12 text-center">
          <Building2 className="mx-auto text-faint mb-3" />
          <p className="text-sm text-muted-foreground">No workspaces found.</p>
        </div>
      )}
      <div className="overflow-x-auto -mx-4 sm:mx-0">
        <div className="min-w-[680px] px-4 sm:px-0">
          <div className="bg-surface border border-border rounded-xl overflow-hidden">
            {workspaces.map((ws) => (
              <div key={ws.id} className="flex items-center gap-4 px-4 py-3 border-b last:border-b-0 border-gray-50 text-[12px] hover:bg-gray-50/50">
                {/* Name + slug */}
                <div className="flex-1 min-w-0">
                  <p className="font-medium text-foreground truncate">{ws.display_name}</p>
                  <p className="text-[10px] text-faint font-mono">{ws.name}</p>
                </div>
                {/* Stats */}
                <div className="flex items-center gap-4 text-[11px] text-muted-foreground flex-shrink-0">
                  <span className="flex items-center gap-1" title="Agents">
                    <Bot className="w-3 h-3" />{ws.agent_count}
                  </span>
                  <span className="flex items-center gap-1" title="Runs">
                    <Activity className="w-3 h-3" />{ws.run_count.toLocaleString()}
                  </span>
                  <span className="flex items-center gap-1 hidden sm:flex" title="Tokens">
                    <Coins className="w-3 h-3" />{formatTokens(ws.total_tokens)}
                  </span>
                  <span className="text-faint hidden md:block">{relativeTime(ws.created_at)}</span>
                </div>
                {/* Delete */}
                <button
                  onClick={() => { setConfirmDelete(ws); setDeleteError('') }}
                  className="p-1.5 rounded hover:bg-red-50 text-faint hover:text-red-500 transition-colors flex-shrink-0"
                  title="Delete workspace"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Delete confirmation modal */}
      {confirmDelete && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
          <div className="bg-surface rounded-xl shadow-lg w-full max-w-md mx-4 p-6">
            <h3 className="text-base font-semibold text-foreground mb-1">Delete workspace?</h3>
            <p className="text-sm text-muted-foreground mb-1">
              You are about to permanently delete <strong>{confirmDelete.display_name}</strong>.
            </p>
            <p className="text-xs text-amber-600 dark:text-amber-300 bg-amber-50 dark:bg-amber-500/10 border border-amber-100 rounded-lg p-3 mb-4">
              This will cascade-delete all agents, runs, connectors, skills, tools, and data associated with this workspace. This cannot be undone.
            </p>
            {deleteError && (
              <p className="text-xs text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-100 rounded-lg p-2 mb-3">{deleteError}</p>
            )}
            <div className="flex gap-2 justify-end">
              <button
                onClick={() => setConfirmDelete(null)}
                className="px-4 py-2 text-sm text-muted-foreground border border-border-strong rounded-lg hover:bg-muted"
              >
                Cancel
              </button>
              <button
                onClick={() => deleteMut.mutate(confirmDelete.id)}
                disabled={deleteMut.isPending}
                className="px-4 py-2 text-sm bg-red-600 text-white rounded-lg hover:bg-red-700 disabled:opacity-50"
              >
                {deleteMut.isPending ? 'Deleting…' : 'Delete workspace'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
