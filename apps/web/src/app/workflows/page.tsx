'use client'

import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { GitBranch, ExternalLink, Plus, Trash2 } from 'lucide-react'
import { workflowsAPI } from '@/lib/api'
import { relativeTime, statusColor } from '@/lib/utils'
import { PageHeader } from '@/components/ui/PageHeader'
import type { Workflow } from '@/types'

export default function WorkflowsPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { data, isLoading, error } = useQuery({ queryKey: ['workflows'], queryFn: () => workflowsAPI.list() as Promise<{ data: Workflow[] }> })
  const remove = useMutation({ mutationFn: (id: string) => workflowsAPI.delete(id), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['workflows'] }) })
  const workflows = data?.data ?? []

  return <div className="p-4 sm:p-6 max-w-6xl">
    <PageHeader
      eyebrow="Build"
      title="Workflows"
      subtitle="Visual multi-agent orchestration — pipelines and supervisors."
      actions={
        <Link href="/workflows/new" className="inline-flex items-center gap-1.5 px-3.5 py-2 bg-gradient-to-br from-accent to-accent-ink text-white text-[13px] font-semibold rounded-[10px] shadow-card hover:opacity-95">
          <Plus className="w-4 h-4" /> New workflow
        </Link>
      }
    />
    {error && <div className="text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg p-3 mb-4">{(error as Error).message}</div>}
    {isLoading && <div className="py-12 text-center text-sm text-faint">Loading workflows…</div>}
    {!isLoading && !error && workflows.length === 0 && (
      <div className="border border-dashed border-border-strong rounded-xl py-12 text-center">
        <GitBranch className="mx-auto text-faint mb-3" />
        <p className="text-sm text-muted-foreground">No workflows yet.</p>
      </div>
    )}
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
      {workflows.map((wf) => (
        <div key={wf.id} className="bg-surface border border-border rounded-xl p-4">
          <div className="flex items-start justify-between">
            <div>
              <p className="text-[13px] font-medium text-foreground">{wf.name}</p>
              <p className="text-[11px] text-faint mt-1">{wf.description || `${wf.mode} workflow`}</p>
            </div>
            <span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(wf.status ?? 'active')}`}>{wf.status ?? 'active'}</span>
          </div>
          <p className="text-[11px] text-muted-foreground mt-3">
            {wf.mode} workflow · {wf.run_count ?? 0} runs
          </p>
          <div className="flex gap-2 mt-4 pt-3 border-t border-gray-50">
            <span className="text-[10px] text-faint mr-auto">
              {wf.last_run_at ? `Last run ${relativeTime(wf.last_run_at)}` : 'Never run'}
            </span>
            <button
              onClick={() => router.push(`/workflows/${wf.id}`)}
              className="inline-flex items-center gap-1 px-2.5 py-1 bg-accent text-white text-[11px] rounded-md"
            >
              <ExternalLink className="w-2.5 h-2.5" /> Open
            </button>
            <button
              onClick={() => { if (confirm('Delete this workflow?')) remove.mutate(wf.id) }}
              className="p-1 text-faint hover:text-red-500"
            >
              <Trash2 className="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      ))}
    </div>

    <div className="mt-6 p-4 rounded-lg bg-muted border border-border-strong">
      <p className="text-sm text-muted-foreground">
        Learn how to build multi-agent workflows in the{' '}
        <Link href="/docs/what-is-an-agent-group" className="text-accent dark:text-accent-bright hover:underline">
          documentation
        </Link>.
      </p>
    </div>
  </div>
}
