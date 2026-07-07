'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, GitBranch, Loader2, Plus, Trash2 } from 'lucide-react'
import { agentsAPI, pipelineAPI, repoCatalogAPI, runnerCredsAPI, runsAPI, webhookTriggersAPI } from '@/lib/api'
import { relativeTime, statusColor, cn } from '@/lib/utils'
import { useAuthStore } from '@/store/auth'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { Agent, Run } from '@/types'

type RunnerCreds = {
  claude_connected: boolean
  github_connected: boolean
  jira_connected: boolean
  github_env_fallback: boolean
}

type CatalogRepo = {
  repo: string
  default_branch: string
  sessions_enabled: boolean
  documents: number
  chunks: number
  updated_at: string
}

const PIPELINE_AGENT_NAMES = ['Jira Pipeline Orchestrator', 'Code Review Agent', 'Docs Map Maintainer']

function ReadinessBadge({ label, ok, hint }: { label: string; ok: boolean; hint: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex items-center gap-1.5 cursor-default rounded-full border border-border bg-surface px-2.5 py-1 text-[11px] font-medium text-muted-foreground">
          <span className={cn('w-1.5 h-1.5 rounded-full flex-shrink-0', ok ? 'bg-good' : 'bg-faint')} />
          {label}
        </span>
      </TooltipTrigger>
      <TooltipContent>{hint}</TooltipContent>
    </Tooltip>
  )
}

// Home for the repo-session ("Claude Code") pipeline: readiness, the repo
// allowlist, and recent session activity. Credentials live in
// Settings → Claude Code (same pattern as Providers/API Tokens); this page
// owns everything else so it isn't scattered across a single Settings leaf.
// Readiness is a compact hover-for-detail strip rather than a tall checklist,
// and Repositories/Sessions live in tabs instead of stacked cards, so the
// page reads as one screen instead of three independent panels.
export default function ClaudeCodePage() {
  const queryClient = useQueryClient()
  const { user } = useAuthStore()

  const { data: creds } = useQuery({
    queryKey: ['runner-credentials'],
    queryFn: () => runnerCredsAPI.get() as Promise<RunnerCreds>,
  })
  const { data: agentsData } = useQuery({
    queryKey: ['agents'],
    queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }>,
  })
  const { data: catalogData } = useQuery({
    queryKey: ['repo-catalog'],
    queryFn: () => repoCatalogAPI.list() as Promise<{ data: CatalogRepo[] }>,
  })
  const { data: pipeStatus } = useQuery({
    queryKey: ['pipeline-status'],
    queryFn: () => pipelineAPI.status() as Promise<{ runner_configured: boolean; runner_reachable: boolean; runner_executor: string }>,
    refetchInterval: 30000,
  })
  const { data: triggersData } = useQuery({
    queryKey: ['webhook-triggers'],
    queryFn: () => webhookTriggersAPI.list() as Promise<{ data: { id: string; name: string; is_active: boolean; target_id: string }[] }>,
  })

  const agents = agentsData?.data ?? []
  const orchestrator = agents.find((a) => a.name === 'Jira Pipeline Orchestrator')
  const seededAgents = PIPELINE_AGENT_NAMES.filter((n) => agents.some((a) => a.name === n))
  const repos = catalogData?.data ?? []
  const enabledRepos = repos.filter((r) => r.sessions_enabled)
  const hasCatalog = enabledRepos.length > 0
  const triggers = triggersData?.data ?? []
  const triggerCount = triggers.filter((t) => t.is_active).length
  const pipelineTrigger = triggers.find((t) => t.target_id === orchestrator?.id)

  const { data: sessionsData, isLoading: sessionsLoading } = useQuery({
    queryKey: ['claude-code-sessions', orchestrator?.id],
    queryFn: () => runsAPI.listPage({ agent_id: orchestrator!.id }) as Promise<{ data: Run[] }>,
    enabled: !!orchestrator?.id,
  })
  const sessions = (sessionsData?.data ?? []).slice(0, 8)

  const [newRepo, setNewRepo] = useState('')
  const [repoError, setRepoError] = useState('')
  const onboard = useMutation({
    mutationFn: (repo: string) => repoCatalogAPI.onboard(repo),
    onSuccess: () => {
      setNewRepo('')
      setRepoError('')
      queryClient.invalidateQueries({ queryKey: ['repo-catalog'] })
    },
    onError: (err: Error) => setRepoError(err.message),
  })
  const removeRepo = useMutation({
    mutationFn: (repo: string) => repoCatalogAPI.remove(repo),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['repo-catalog'] }),
  })
  const toggleSessions = useMutation({
    mutationFn: (r: { repo: string; enabled: boolean }) => repoCatalogAPI.setSessions(r.repo, r.enabled),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['repo-catalog'] }),
  })

  const runnerHint = !pipeStatus?.runner_configured
    ? 'RUNNER_URL is not set on the API — sessions are disabled'
    : !pipeStatus?.runner_reachable
      ? 'Runner is configured but not responding'
      : pipeStatus.runner_executor === 'stub'
        ? '⚠ STUB MODE — sessions are simulated: no code is written, branches are fake. Set RUNNER_EXECUTOR=claude for real sessions.'
        : 'Real Claude Code sessions enabled'

  const readiness: { label: string; ok: boolean; hint: string }[] = [
    {
      label: `Runner${pipeStatus?.runner_executor ? ` · ${pipeStatus.runner_executor}` : ''}`,
      ok: !!pipeStatus?.runner_reachable && pipeStatus?.runner_executor !== 'stub',
      hint: runnerHint,
    },
    {
      label: 'Claude account',
      ok: !!creds?.claude_connected,
      hint: creds?.claude_connected ? 'Coding sessions bill your Claude subscription' : 'Connect in Settings → Claude Code',
    },
    {
      label: 'GitHub token',
      ok: !!creds?.github_connected || !!creds?.github_env_fallback,
      hint: creds?.github_connected
        ? 'Using this workspace’s token'
        : creds?.github_env_fallback
          ? 'Using the instance-level fallback token — set a workspace token in Settings to isolate access'
          : 'Needed for clone, push, and pull requests — connect in Settings → Claude Code',
    },
    {
      label: `Agents ${seededAgents.length}/3`,
      ok: seededAgents.length === 3,
      hint: 'Seeded automatically; protected from deletion — visible in the Agents list',
    },
    {
      label: `Repos ${enabledRepos.length}/${repos.length}`,
      ok: hasCatalog,
      hint: hasCatalog
        ? 'Sessions may only modify enabled repositories'
        : repos.length > 0
          ? 'Repos are indexed but none are session-enabled — flip a toggle in the Repositories tab'
          : 'Add a repository in the Repositories tab, or sync a GitHub connector',
    },
    {
      label: `Triggers ${triggerCount}`,
      ok: triggerCount > 0,
      hint: triggerCount > 0 ? 'Jira/GitHub events will dispatch runs' : 'Run infra/scripts/setup_pipeline.sh',
    },
  ]

  return (
    <TooltipProvider delayDuration={200}>
      <div className="p-4 sm:p-6 max-w-3xl">
        <div className="mb-4">
          <span className="eyebrow block mb-1">Build</span>
          <h1 className="text-[22px] font-bold tracking-tight text-foreground">Claude Code</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            Autonomous repo coding sessions (Jira → PR pipeline) for this workspace.
          </p>
        </div>

        {/* Readiness — a hover-for-detail strip instead of a tall checklist */}
        <div className="flex flex-wrap gap-1.5 mb-5">
          {readiness.map((item) => (
            <ReadinessBadge key={item.label} {...item} />
          ))}
        </div>

        <Tabs defaultValue="repos">
          <TabsList>
            <TabsTrigger value="repos">Repositories</TabsTrigger>
            <TabsTrigger value="sessions">Sessions{sessions.length > 0 ? ` (${sessions.length})` : ''}</TabsTrigger>
          </TabsList>

          <TabsContent value="repos">
            <p className="text-[11px] text-faint mb-3">
              Coding sessions can only target repositories onboarded here. Private repos use this workspace&apos;s
              GitHub token{creds?.github_connected ? '' : ' — connect one in Settings → Claude Code first'}.
            </p>
            {repos.length > 0 && (
              <div className="space-y-1.5 mb-3">
                {repos.map((r) => (
                  <div
                    key={r.repo}
                    className={cn(
                      'flex flex-wrap items-center gap-2 text-[12px] border rounded-lg px-2.5 py-1.5',
                      r.sessions_enabled ? 'border-good/30 bg-good/[0.06]' : 'border-border'
                    )}
                  >
                    <GitBranch size={12} className={r.sessions_enabled ? 'text-good' : 'text-faint'} />
                    <span className="font-mono text-foreground break-all">{r.repo}</span>
                    <span className="text-faint">
                      {r.documents} docs · indexed {relativeTime(r.updated_at)}
                    </span>
                    <div className="ml-auto flex items-center gap-2">
                      <button
                        onClick={() => toggleSessions.mutate({ repo: r.repo, enabled: !r.sessions_enabled })}
                        disabled={toggleSessions.isPending}
                        title={r.sessions_enabled ? 'Sessions may modify this repo — click to revoke' : 'Indexed for context only — click to allow coding sessions'}
                        className={cn(
                          'px-2 py-0.5 rounded-full text-[10px] font-medium border transition-colors disabled:opacity-50',
                          r.sessions_enabled
                            ? 'bg-good/15 text-good border-good/40 hover:bg-good/25'
                            : 'bg-muted text-muted-foreground border-border-strong hover:bg-muted'
                        )}
                      >
                        {r.sessions_enabled ? 'sessions on' : 'enable sessions'}
                      </button>
                      <button
                        onClick={() => { if (confirm(`Remove ${r.repo} from the catalog entirely?`)) removeRepo.mutate(r.repo) }}
                        className="p-1 text-faint hover:text-crit"
                      >
                        <Trash2 size={12} />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
            <div className="flex flex-wrap items-center gap-2">
              <input
                value={newRepo}
                onChange={(e) => setNewRepo(e.target.value)}
                placeholder="owner/repo"
                className="flex-1 min-w-[200px] px-2.5 py-1.5 border border-border-strong rounded-lg text-xs font-mono focus:outline-none focus:ring-1 focus:ring-accent"
                onKeyDown={(e) => { if (e.key === 'Enter' && newRepo.trim() && !onboard.isPending) onboard.mutate(newRepo.trim()) }}
              />
              <button
                onClick={() => onboard.mutate(newRepo.trim())}
                disabled={!newRepo.trim() || onboard.isPending}
                className="inline-flex items-center gap-1 px-3 py-1.5 bg-accent text-white text-xs rounded-lg font-medium disabled:opacity-50"
              >
                {onboard.isPending ? <Loader2 size={12} className="animate-spin" /> : <Plus size={12} />}
                {onboard.isPending ? 'Cloning & indexing…' : 'Add repository'}
              </button>
            </div>
            {repoError && <p className="text-xs text-crit mt-2">{repoError}</p>}
          </TabsContent>

          <TabsContent value="sessions">
            {!orchestrator ? (
              <p className="text-[12px] text-faint py-3">
                The Jira Pipeline Orchestrator agent hasn&apos;t been seeded for this workspace yet.
              </p>
            ) : sessionsLoading ? (
              <p className="text-[12px] text-faint py-3">Loading…</p>
            ) : sessions.length === 0 ? (
              <p className="text-[12px] text-faint py-3 flex items-center gap-1">
                <Activity size={12} />
                No sessions yet — trigger one from Jira{pipelineTrigger ? '' : ', or '}
                {!pipelineTrigger && <Link href="/triggers/new" className="text-accent dark:text-accent-bright hover:underline">create a webhook trigger</Link>}
                {!pipelineTrigger ? ' for the orchestrator agent.' : '.'}
              </p>
            ) : (
              <div className="space-y-1.5">
                {sessions.map((run) => (
                  <Link
                    key={run.id}
                    href={`/runs/${run.id}`}
                    className="flex items-center gap-2.5 text-[12px] border border-border rounded-lg px-2.5 py-1.5 hover:bg-muted"
                  >
                    <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${statusColor(run.status)}`}>{run.status}</span>
                    <span className="font-mono text-muted-foreground">{run.id.slice(0, 8)}</span>
                    <span className="text-faint ml-auto">{relativeTime(run.started_at)}</span>
                  </Link>
                ))}
              </div>
            )}
          </TabsContent>
        </Tabs>

        <p className="text-[11px] text-faint mt-5">
          Missing a credential? <Link href="/settings/claude-code" className="text-accent dark:text-accent-bright hover:underline">Settings → Claude Code</Link>.
          {user?.is_admin && (
            <> Instance-wide health: <Link href="/admin/claude-code" className="text-accent dark:text-accent-bright hover:underline">Admin → Claude Code</Link>.</>
          )}
        </p>
      </div>
    </TooltipProvider>
  )
}
