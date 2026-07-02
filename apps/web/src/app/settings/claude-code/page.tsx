'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, GitBranch, Terminal, Unplug, Workflow } from 'lucide-react'
import { agentsAPI, connectorsAPI, runnerCredsAPI, webhookTriggersAPI } from '@/lib/api'
import { relativeTime } from '@/lib/utils'
import type { Agent } from '@/types'

type RunnerCreds = {
  claude_connected: boolean
  github_connected: boolean
  github_env_fallback: boolean
  updated_at?: string
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
  const { data: connectorsData } = useQuery({
    queryKey: ['connectors'],
    queryFn: () => connectorsAPI.list() as Promise<{ data: { name: string }[] }>,
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

  const agents = agentsData?.data ?? []
  const seededAgents = PIPELINE_AGENT_NAMES.filter((n) => agents.some((a) => a.name === n))
  const hasCatalog = (connectorsData?.data ?? []).some((c) => c.name === 'repo-catalog')
  const triggerCount = (triggersData?.data ?? []).filter((t) => t.is_active).length

  const checklist: { label: string; ok: boolean; hint: string }[] = [
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
      label: 'Repo catalog',
      ok: hasCatalog,
      hint: hasCatalog ? 'Connector present' : 'Onboard repos with catalog-ingest',
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
