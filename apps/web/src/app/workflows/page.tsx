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
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3.5">
      {workflows.map((wf) => (
        <div key={wf.id} className="group bg-surface border border-border shadow-card rounded-xl p-4 flex flex-col gap-3 hover:border-border-strong hover:-translate-y-0.5 transition-all">
          <div className="flex items-center gap-2.5">
            <div className="w-9 h-9 rounded-lg bg-accent/10 dark:bg-accent-bright/10 text-accent dark:text-accent-bright grid place-items-center flex-shrink-0">
              <GitBranch className="w-4 h-4" />
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-[13px] font-semibold text-foreground truncate">{wf.name}</p>
              <p className="font-mono text-[10px] text-faint">{wf.run_count ?? 0} runs</p>
            </div>
          </div>
          <p className="text-[12px] text-muted-foreground line-clamp-2 min-h-[2rem]">{wf.description || `${wf.mode} workflow`}</p>
          <div className="flex items-center gap-1.5">
            <span className="font-mono text-[10px] px-2 py-0.5 rounded-full bg-accent/10 text-accent dark:text-accent-bright border border-accent/25">{wf.mode}</span>
            <span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(wf.status ?? 'active')}`}>{wf.status ?? 'active'}</span>
          </div>
          <div className="flex items-center gap-2 mt-1 pt-3 border-t border-border">
            <span className="font-mono text-[10px] text-faint mr-auto">
              {wf.last_run_at ? `last run ${relativeTime(wf.last_run_at)}` : 'never run'}
            </span>
            <button onClick={() => router.push(`/workflows/${wf.id}`)} className="inline-flex items-center gap-1 px-2.5 py-1 bg-accent text-white text-[11px] rounded-lg hover:bg-accent-hover">
              <ExternalLink className="w-2.5 h-2.5" /> Open
            </button>
            <button onClick={() => { if (confirm('Delete this workflow?')) remove.mutate(wf.id) }} className="p-1 text-faint hover:text-crit"><Trash2 className="w-3.5 h-3.5" /></button>
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
