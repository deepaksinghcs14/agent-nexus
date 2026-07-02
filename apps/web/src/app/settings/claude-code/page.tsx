'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { BookMarked, Check, GitBranch, Loader2, Plus, Terminal, Trash2, Unplug, Workflow } from 'lucide-react'
import { agentsAPI, pipelineAPI, repoCatalogAPI, runnerCredsAPI, webhookTriggersAPI } from '@/lib/api'
import { relativeTime } from '@/lib/utils'
import type { Agent } from '@/types'

type RunnerCreds = {
  claude_connected: boolean
  github_connected: boolean
  github_env_fallback: boolean
  updated_at?: string
}

type CatalogRepo = {
  repo: string
  default_branch: string
  sessions_enabled: boolean
  documents: number
  chunks: number
  updated_at: string
}

// Settings → Claude Code: everything the Jira→PR pipeline needs, configured
// per workspace from the UI. Credentials are stored encrypted, never shown
// again, and injected per coding session — a workspace GitHub token takes
// precedence over any instance-level GITHUB_TOKEN env.

function CredentialCard(props: {
  icon: React.ReactNode
  title: string
  connected: boolean
  connectedNote: string
  disconnectedNote: React.ReactNode
  placeholder: string
  footnote: string
  onSave: (token: string) => void
  onDisconnect: () => void
  saving: boolean
  error: string
}) {
  const [token, setToken] = useState('')
  return (
    <div className="border border-gray-200 bg-white rounded-xl px-4 py-3">
      <div className="flex flex-wrap items-center gap-3 justify-between">
        <div className="flex items-center gap-2.5">
          {props.icon}
          <div>
            <p className="text-sm font-medium text-gray-900">{props.title}</p>
            {props.connected ? (
              <p className="text-xs text-green-700 flex items-center gap-1">
                <Check size={11} /> {props.connectedNote}
              </p>
            ) : (
              <p className="text-xs text-gray-500">{props.disconnectedNote}</p>
            )}
          </div>
        </div>
        {props.connected && (
          <button
            onClick={props.onDisconnect}
            className="inline-flex items-center gap-1 px-2.5 py-1.5 border border-gray-200 text-xs text-gray-700 rounded-lg hover:bg-gray-50"
          >
            <Unplug size={12} /> Disconnect
          </button>
        )}
      </div>
      {!props.connected && (
        <div className="flex flex-wrap items-center gap-2 mt-2.5">
          <input
            type="password"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder={props.placeholder}
            className="flex-1 min-w-[220px] px-2.5 py-1.5 border border-gray-200 rounded-lg text-xs font-mono focus:outline-none focus:ring-1 focus:ring-purple-400"
          />
          <button
            onClick={() => { props.onSave(token.trim()); setToken('') }}
            disabled={!token.trim() || props.saving}
            className="px-3 py-1.5 bg-purple-600 text-white text-xs rounded-lg font-medium disabled:opacity-50"
          >
            {props.saving ? 'Saving…' : 'Connect'}
          </button>
        </div>
      )}
      {props.error && <p className="text-xs text-red-600 mt-2">{props.error}</p>}
      <p className="text-[11px] text-gray-400 mt-2">{props.footnote}</p>
    </div>
  )
}

const PIPELINE_AGENT_NAMES = ['Jira Pipeline Orchestrator', 'Code Review Agent', 'Docs Map Maintainer']

export default function ClaudeCodePage() {
  const queryClient = useQueryClient()
  const [error, setError] = useState('')

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
    queryFn: () => webhookTriggersAPI.list() as Promise<{ data: { name: string; is_active: boolean }[] }>,
  })

  const save = useMutation({
    mutationFn: (body: { claude_token?: string; github_token?: string }) => runnerCredsAPI.put(body),
    onSuccess: () => { setError(''); queryClient.invalidateQueries({ queryKey: ['runner-credentials'] }) },
    onError: (err: Error) => setError(err.message),
  })
  const disconnect = useMutation({
    mutationFn: (field: 'claude' | 'github') => runnerCredsAPI.delete(field),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['runner-credentials'] }),
  })

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

  const agents = agentsData?.data ?? []
  const seededAgents = PIPELINE_AGENT_NAMES.filter((n) => agents.some((a) => a.name === n))
  const repos = catalogData?.data ?? []
  const enabledRepos = repos.filter((r) => r.sessions_enabled)
  const hasCatalog = enabledRepos.length > 0
  const triggerCount = (triggersData?.data ?? []).filter((t) => t.is_active).length

  const runnerHint = !pipeStatus?.runner_configured
    ? 'RUNNER_URL is not set on the API — sessions are disabled'
    : !pipeStatus?.runner_reachable
      ? 'Runner is configured but not responding'
      : pipeStatus.runner_executor === 'stub'
        ? '⚠ STUB MODE — sessions are simulated: no code is written, branches are fake. Set RUNNER_EXECUTOR=claude for real sessions.'
        : 'Real Claude Code sessions enabled'

  const checklist: { label: string; ok: boolean; hint: string }[] = [
    {
      label: `Runner${pipeStatus?.runner_executor ? ` (${pipeStatus.runner_executor} mode)` : ''}`,
      ok: !!pipeStatus?.runner_reachable && pipeStatus?.runner_executor !== 'stub',
      hint: runnerHint,
    },
    {
      label: 'Claude account',
      ok: !!creds?.claude_connected,
      hint: 'Coding sessions bill your Claude subscription',
    },
    {
      label: 'GitHub token',
      ok: !!creds?.github_connected || !!creds?.github_env_fallback,
      hint: creds?.github_connected
        ? 'Using this workspace’s token'
        : creds?.github_env_fallback
          ? 'Using the instance-level fallback token — set a workspace token to isolate access'
          : 'Needed for clone, push, and pull requests',
    },
    {
      label: `Pipeline agents (${seededAgents.length}/3)`,
      ok: seededAgents.length === 3,
      hint: 'Seeded automatically; protected from deletion',
    },
    {
      label: `Repositories (${enabledRepos.length} enabled / ${repos.length} known)`,
      ok: hasCatalog,
      hint: hasCatalog
        ? 'Sessions may only modify enabled repositories'
        : repos.length > 0
          ? 'Repos are indexed but none are session-enabled — flip a toggle below'
          : 'Add a repository below, or sync a GitHub connector',
    },
    {
      label: `Webhook triggers (${triggerCount} active)`,
      ok: triggerCount > 0,
      hint: triggerCount > 0 ? 'Jira/GitHub events will dispatch runs' : 'Run infra/scripts/setup_pipeline.sh',
    },
  ]

  return (
    <div className="p-4 sm:p-6 max-w-2xl">
      <div className="mb-5">
        <h1 className="text-xl font-semibold text-gray-900">Claude Code</h1>
        <p className="text-sm text-gray-500 mt-0.5">
          Workspace settings for autonomous repo coding sessions (Jira → PR pipeline). Credentials are
          encrypted at rest and injected per session.
        </p>
      </div>

      <div className="space-y-3 mb-6">
        <CredentialCard
          icon={<Terminal size={16} className="text-amber-700" />}
          title="Claude account"
          connected={!!creds?.claude_connected}
          connectedNote={`Connected · subscription billing${creds?.updated_at ? ` · updated ${relativeTime(creds.updated_at)}` : ''}`}
          disconnectedNote={
            <>Run <code className="bg-gray-100 px-1 rounded">claude setup-token</code> in your terminal (opens the usual Claude login), then paste the token</>
          }
          placeholder="sk-ant-oat…"
          footnote="Authenticates the coding sessions. Without it, the runner falls back to its own ANTHROPIC_API_KEY."
          onSave={(t) => save.mutate({ claude_token: t })}
          onDisconnect={() => disconnect.mutate('claude')}
          saving={save.isPending}
          error={error}
        />
        <CredentialCard
          icon={<GitBranch size={16} className="text-gray-800" />}
          title="GitHub token"
          connected={!!creds?.github_connected}
          connectedNote={`Connected · used for clone, push, and PRs${creds?.updated_at ? ` · updated ${relativeTime(creds.updated_at)}` : ''}`}
          disconnectedNote={
            <>Fine-grained PAT or classic token with <code className="bg-gray-100 px-1 rounded">repo</code> scope for the repositories this workspace works on</>
          }
          placeholder="ghp_… or github_pat_…"
          footnote="Scoped to this workspace — other workspaces never see it. Takes precedence over any instance-level GITHUB_TOKEN."
          onSave={(t) => save.mutate({ github_token: t })}
          onDisconnect={() => disconnect.mutate('github')}
          saving={save.isPending}
          error={error}
        />
      </div>

      {/* Repositories: the allowlist of repos coding sessions may target.
          Onboarding clones + indexes server-side with the workspace token. */}
      <div className="border border-gray-200 bg-white rounded-xl px-4 py-3 mb-6">
        <div className="flex items-center gap-2 mb-1">
          <BookMarked size={15} className="text-purple-600" />
          <p className="text-sm font-medium text-gray-900">Repositories</p>
        </div>
        <p className="text-[11px] text-gray-400 mb-3">
          Coding sessions can only target repositories onboarded here. Private repos use this workspace&apos;s
          GitHub token{creds?.github_connected ? '' : ' — connect one above first'}.
        </p>
        {repos.length > 0 && (
          <div className="space-y-1.5 mb-3">
            {repos.map((r) => (
              <div
                key={r.repo}
                className={`flex flex-wrap items-center gap-2 text-[12px] border rounded-lg px-2.5 py-1.5 ${
                  r.sessions_enabled ? 'border-green-200 bg-green-50/40' : 'border-gray-100'
                }`}
              >
                <GitBranch size={12} className={r.sessions_enabled ? 'text-green-600' : 'text-gray-400'} />
                <span className="font-mono text-gray-800">{r.repo}</span>
                <span className="text-gray-400">
                  {r.documents} docs · indexed {relativeTime(r.updated_at)}
                </span>
                <div className="ml-auto flex items-center gap-2">
                  <button
                    onClick={() => toggleSessions.mutate({ repo: r.repo, enabled: !r.sessions_enabled })}
                    disabled={toggleSessions.isPending}
                    title={r.sessions_enabled ? 'Sessions may modify this repo — click to revoke' : 'Indexed for context only — click to allow coding sessions'}
                    className={`px-2 py-0.5 rounded-full text-[10px] font-medium border transition-colors disabled:opacity-50 ${
                      r.sessions_enabled
                        ? 'bg-green-100 text-green-800 border-green-300 hover:bg-green-200'
                        : 'bg-gray-100 text-gray-500 border-gray-200 hover:bg-gray-200'
                    }`}
                  >
                    {r.sessions_enabled ? 'sessions on' : 'enable sessions'}
                  </button>
                  <button
                    onClick={() => { if (confirm(`Remove ${r.repo} from the catalog entirely?`)) removeRepo.mutate(r.repo) }}
                    className="p-1 text-gray-300 hover:text-red-500"
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
            className="flex-1 min-w-[200px] px-2.5 py-1.5 border border-gray-200 rounded-lg text-xs font-mono focus:outline-none focus:ring-1 focus:ring-purple-400"
            onKeyDown={(e) => { if (e.key === 'Enter' && newRepo.trim() && !onboard.isPending) onboard.mutate(newRepo.trim()) }}
          />
          <button
            onClick={() => onboard.mutate(newRepo.trim())}
            disabled={!newRepo.trim() || onboard.isPending}
            className="inline-flex items-center gap-1 px-3 py-1.5 bg-purple-600 text-white text-xs rounded-lg font-medium disabled:opacity-50"
          >
            {onboard.isPending ? <Loader2 size={12} className="animate-spin" /> : <Plus size={12} />}
            {onboard.isPending ? 'Cloning & indexing…' : 'Add repository'}
          </button>
        </div>
        {repoError && <p className="text-xs text-red-600 mt-2">{repoError}</p>}
      </div>

      <div className="border border-gray-200 bg-white rounded-xl px-4 py-3">
        <div className="flex items-center gap-2 mb-3">
          <Workflow size={15} className="text-purple-600" />
          <p className="text-sm font-medium text-gray-900">Pipeline readiness</p>
        </div>
        <div className="space-y-2">
          {checklist.map((item) => (
            <div key={item.label} className="flex items-start gap-2.5">
              <span
                className={`mt-0.5 w-4 h-4 rounded-full flex items-center justify-center flex-shrink-0 text-[9px] font-bold ${
                  item.ok ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-400'
                }`}
              >
                {item.ok ? '✓' : '·'}
              </span>
              <div>
                <p className="text-[12px] font-medium text-gray-800">{item.label}</p>
                <p className="text-[11px] text-gray-400">{item.hint}</p>
              </div>
            </div>
          ))}
        </div>
        <p className="text-[11px] text-gray-400 mt-3">
          Remaining external steps (Atlassian OAuth on the MCP Servers page, Jira/GitHub webhook URLs) are
          covered in docs/jira-pipeline.md.
        </p>
      </div>
    </div>
  )
}
