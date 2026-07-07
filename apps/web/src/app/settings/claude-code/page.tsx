'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, ClipboardList, GitBranch, Terminal, Unplug } from 'lucide-react'
import { runnerCredsAPI } from '@/lib/api'
import { relativeTime } from '@/lib/utils'

type RunnerCreds = {
  claude_connected: boolean
  github_connected: boolean
  jira_connected: boolean
  github_env_fallback: boolean
  jira_env_fallback: boolean
  jira_base_url?: string
  updated_at?: string
}

// Settings → Claude Code: workspace credentials for the Jira→PR repo-session
// pipeline (Claude account, GitHub token, Jira). Encrypted at rest, injected
// per coding session — a workspace GitHub token takes precedence over any
// instance-level GITHUB_TOKEN env. Everything else (repo allowlist,
// readiness, session activity) lives on the Claude Code nav page.

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
    <div className="border border-border-strong bg-surface rounded-xl px-4 py-3">
      <div className="flex flex-wrap items-center gap-3 justify-between">
        <div className="flex items-center gap-2.5">
          {props.icon}
          <div>
            <p className="text-sm font-medium text-foreground">{props.title}</p>
            {props.connected ? (
              <p className="text-xs text-green-700 dark:text-green-300 flex items-center gap-1">
                <Check size={11} /> {props.connectedNote}
              </p>
            ) : (
              <p className="text-xs text-muted-foreground">{props.disconnectedNote}</p>
            )}
          </div>
        </div>
        {props.connected && (
          <button
            onClick={props.onDisconnect}
            className="inline-flex items-center gap-1 px-2.5 py-1.5 border border-border-strong text-xs text-foreground rounded-lg hover:bg-muted"
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
            className="flex-1 min-w-[220px] px-2.5 py-1.5 border border-border-strong rounded-lg text-xs font-mono focus:outline-none focus:ring-1 focus:ring-accent"
          />
          <button
            onClick={() => { props.onSave(token.trim()); setToken('') }}
            disabled={!token.trim() || props.saving}
            className="px-3 py-1.5 bg-accent text-white text-xs rounded-lg font-medium disabled:opacity-50"
          >
            {props.saving ? 'Saving…' : 'Connect'}
          </button>
        </div>
      )}
      {props.error && <p className="text-xs text-red-600 dark:text-red-300 mt-2">{props.error}</p>}
      <p className="text-[11px] text-faint mt-2">{props.footnote}</p>
    </div>
  )
}

// Jira needs three fields (site URL, email, API token), so it gets its own
// card instead of CredentialCard's single-token layout. Email stays optional:
// Data Center PATs authenticate with Bearer and no email.
function JiraCredentialCard(props: {
  connected: boolean
  envFallback: boolean
  baseURL?: string
  updatedAt?: string
  onSave: (creds: { jira_base_url: string; jira_email: string; jira_api_token: string }) => void
  onDisconnect: () => void
  saving: boolean
  error: string
}) {
  const [baseURL, setBaseURL] = useState('')
  const [email, setEmail] = useState('')
  const [token, setToken] = useState('')
  const canSave = baseURL.trim() !== '' && token.trim() !== ''
  return (
    <div className="border border-border-strong bg-surface rounded-xl px-4 py-3">
      <div className="flex flex-wrap items-center gap-3 justify-between">
        <div className="flex items-center gap-2.5">
          <ClipboardList size={16} className="text-blue-700 dark:text-blue-300" />
          <div>
            <p className="text-sm font-medium text-foreground">Jira</p>
            {props.connected ? (
              <p className="text-xs text-green-700 dark:text-green-300 flex items-center gap-1">
                <Check size={11} /> Connected · {props.baseURL}
                {props.updatedAt ? ` · updated ${relativeTime(props.updatedAt)}` : ''}
              </p>
            ) : (
              <p className="text-xs text-muted-foreground">
                API-token auth for the native Jira tools (issues, JQL search, comments, transitions)
                {props.envFallback ? ' — instance-level JIRA_* env is active as fallback' : ''}
              </p>
            )}
          </div>
        </div>
        {props.connected && (
          <button
            onClick={props.onDisconnect}
            className="inline-flex items-center gap-1 px-2.5 py-1.5 border border-border-strong text-xs text-foreground rounded-lg hover:bg-muted"
          >
            <Unplug size={12} /> Disconnect
          </button>
        )}
      </div>
      {!props.connected && (
        <div className="flex flex-col gap-2 mt-2.5">
          <input
            type="url"
            value={baseURL}
            onChange={(e) => setBaseURL(e.target.value)}
            placeholder="https://yourorg.atlassian.net"
            className="w-full px-2.5 py-1.5 border border-border-strong rounded-lg text-xs font-mono focus:outline-none focus:ring-1 focus:ring-accent"
          />
          <div className="flex flex-wrap items-center gap-2">
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@org.com (leave empty for Data Center PAT)"
              className="flex-1 min-w-[220px] px-2.5 py-1.5 border border-border-strong rounded-lg text-xs font-mono focus:outline-none focus:ring-1 focus:ring-accent"
            />
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="API token"
              className="flex-1 min-w-[180px] px-2.5 py-1.5 border border-border-strong rounded-lg text-xs font-mono focus:outline-none focus:ring-1 focus:ring-accent"
            />
            <button
              onClick={() => {
                props.onSave({ jira_base_url: baseURL.trim(), jira_email: email.trim(), jira_api_token: token.trim() })
                setToken('')
              }}
              disabled={!canSave || props.saving}
              className="px-3 py-1.5 bg-accent text-white text-xs rounded-lg font-medium disabled:opacity-50"
            >
              {props.saving ? 'Saving…' : 'Connect'}
            </button>
          </div>
        </div>
      )}
      {props.error && <p className="text-xs text-red-600 dark:text-red-300 mt-2">{props.error}</p>}
      <p className="text-[11px] text-faint mt-2">
        Cloud: create an API token at id.atlassian.com → Security → API tokens, and enter your account email.
        Data Center: use a personal access token and leave email empty.
      </p>
    </div>
  )
}

export default function ClaudeCodePage() {
  const queryClient = useQueryClient()
  const [error, setError] = useState('')

  const { data: creds } = useQuery({
    queryKey: ['runner-credentials'],
    queryFn: () => runnerCredsAPI.get() as Promise<RunnerCreds>,
  })

  const save = useMutation({
    mutationFn: (body: Parameters<typeof runnerCredsAPI.put>[0]) => runnerCredsAPI.put(body),
    onSuccess: () => { setError(''); queryClient.invalidateQueries({ queryKey: ['runner-credentials'] }) },
    onError: (err: Error) => setError(err.message),
  })
  const disconnect = useMutation({
    mutationFn: (field: 'claude' | 'github' | 'jira') => runnerCredsAPI.delete(field),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['runner-credentials'] }),
  })


  return (
    <div className="p-4 sm:p-6 max-w-2xl">
      <div className="mb-5">
        <span className="eyebrow block mb-1">Settings</span>
          <h1 className="text-[22px] font-bold tracking-tight text-foreground">Claude Code</h1>
        <p className="text-sm text-muted-foreground mt-0.5">
          Credentials for autonomous repo coding sessions (Jira → PR pipeline). Encrypted at rest and
          injected per session.
        </p>
      </div>

      <div className="space-y-3 mb-6">
        <CredentialCard
          icon={<Terminal size={16} className="text-amber-700 dark:text-amber-300" />}
          title="Claude account"
          connected={!!creds?.claude_connected}
          connectedNote={`Connected · subscription billing${creds?.updated_at ? ` · updated ${relativeTime(creds.updated_at)}` : ''}`}
          disconnectedNote={
            <>Run <code className="bg-muted px-1 rounded">claude setup-token</code> in your terminal (opens the usual Claude login), then paste the token</>
          }
          placeholder="sk-ant-oat…"
          footnote="Authenticates the coding sessions. Without it, the runner falls back to its own ANTHROPIC_API_KEY."
          onSave={(t) => save.mutate({ claude_token: t })}
          onDisconnect={() => disconnect.mutate('claude')}
          saving={save.isPending}
          error={error}
        />
        <CredentialCard
          icon={<GitBranch size={16} className="text-foreground" />}
          title="GitHub token"
          connected={!!creds?.github_connected}
          connectedNote={`Connected · used for clone, push, and PRs${creds?.updated_at ? ` · updated ${relativeTime(creds.updated_at)}` : ''}`}
          disconnectedNote={
            <>Fine-grained PAT or classic token with <code className="bg-muted px-1 rounded">repo</code> scope for the repositories this workspace works on</>
          }
          placeholder="ghp_… or github_pat_…"
          footnote="Scoped to this workspace — other workspaces never see it. Takes precedence over any instance-level GITHUB_TOKEN."
          onSave={(t) => save.mutate({ github_token: t })}
          onDisconnect={() => disconnect.mutate('github')}
          saving={save.isPending}
          error={error}
        />
        <JiraCredentialCard
          connected={!!creds?.jira_connected}
          envFallback={!!creds?.jira_env_fallback}
          baseURL={creds?.jira_base_url}
          updatedAt={creds?.updated_at}
          onSave={(c) => save.mutate(c)}
          onDisconnect={() => disconnect.mutate('jira')}
          saving={save.isPending}
          error={error}
        />
      </div>

      <p className="text-[12px] text-muted-foreground">
        Manage repositories and see pipeline readiness / session activity on the{' '}
        <Link href="/claude-code" className="text-accent dark:text-accent-bright hover:underline">Claude Code</Link> page.
      </p>
    </div>
  )
}
