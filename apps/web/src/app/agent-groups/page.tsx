'use client'

import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { GitBranch, ExternalLink, Plus, Trash2 } from 'lucide-react'
import { groupsAPI } from '@/lib/api'
import { relativeTime, statusColor } from '@/lib/utils'
import type { AgentGroup } from '@/types'

export default function AgentGroupsPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useQuery({ queryKey: ['agent-groups'], queryFn: () => groupsAPI.list() as Promise<{ data: AgentGroup[] }> })
  const remove = useMutation({ mutationFn: (id: string) => groupsAPI.delete(id), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agent-groups'] }) })
  const groups = data?.data ?? []

  return <div className="p-6">
    <div className="flex items-center justify-between mb-5">
      <div>
        <h1 className="text-base font-medium text-gray-900">Agent groups</h1>
        <p className="text-[12px] text-gray-400 mt-0.5">Pipeline and supervisor multi-agent workflows</p>
      </div>
      <Link href="/agent-groups/new" className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-[12px] rounded-lg">
        <Plus className="w-3.5 h-3.5" /> New group
      </Link>
    </div>
    {error && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">{(error as Error).message}</div>}
    {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading groups…</div>}
    {!isLoading && !error && groups.length === 0 && (
      <div className="border border-dashed border-gray-200 rounded-xl py-12 text-center">
        <GitBranch className="mx-auto text-gray-300 mb-3" />
        <p className="text-sm text-gray-500">No agent groups yet.</p>
      </div>
    )}
    <div className="grid grid-cols-2 gap-4">
      {groups.map((group) => (
        <div key={group.id} className="bg-white border border-gray-100 rounded-xl p-4">
          <div className="flex items-start justify-between">
            <div>
              <p className="text-[13px] font-medium text-gray-900">{group.name}</p>
              <p className="text-[11px] text-gray-400 mt-1">{group.description || `${group.mode} workflow`}</p>
            </div>
            <span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(group.status ?? 'active')}`}>{group.status ?? 'active'}</span>
          </div>
          <p className="text-[11px] text-gray-500 mt-3">
            {group.mode} workflow · {group.run_count ?? 0} runs
          </p>
          <div className="flex gap-2 mt-4 pt-3 border-t border-gray-50">
            <span className="text-[10px] text-gray-400 mr-auto">
              {group.last_run_at ? `Last run ${relativeTime(group.last_run_at)}` : 'Never run'}
            </span>
            <button
              onClick={() => router.push(`/agent-groups/${group.id}`)}
              className="inline-flex items-center gap-1 px-2.5 py-1 bg-purple-600 text-white text-[11px] rounded-md"
            >
              <ExternalLink className="w-2.5 h-2.5" /> Open
            </button>
            <button
              onClick={() => { if (confirm('Delete this group?')) remove.mutate(group.id) }}
              className="p-1 text-gray-400 hover:text-red-500"
            >
              <Trash2 className="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      ))}
    </div>
  </div>
}
