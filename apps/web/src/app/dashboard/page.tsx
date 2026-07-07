'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useQuery } from '@tanstack/react-query'
import { Activity, Bot, Check, Database, GitBranch, Key, Plug, Sparkles, X, Zap } from 'lucide-react'
import { agentsAPI, connectorsAPI, evalsAPI, gatewayAPI, providersAPI, runsAPI, webhookTriggersAPI, workflowsAPI } from '@/lib/api'
import { useAuthStore } from '@/store/auth'
import { formatTokens, relativeTime, cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/ui/PageHeader'
import { KpiTile } from '@/components/ui/KpiTile'
import type { Connector, Run, WebhookTrigger } from '@/types'

const quickActions = [
  { label: 'Create agent', desc: 'Build a new AI agent', href: '/agents/new', icon: Bot },
  { label: 'Connect MCP', desc: 'Add an MCP server', href: '/mcp-servers', icon: Plug },
  { label: 'Add source', desc: 'Connect a data source', href: '/connectors', icon: Database },
  { label: 'Add provider', desc: 'Configure a model key', href: '/settings/providers', icon: Key },
  { label: 'New trigger', desc: 'Wire a webhook event', href: '/triggers/new', icon: Zap },
]

// Bucket runs into `n` recent equal time windows → a small real series for the
// sparkline. Returns null when there isn't enough data to be meaningful.
function runsSeries(runs: Run[], buckets = 8): number[] | null {
  if (runs.length < buckets) return null
  const times = runs.map((r) => new Date(r.started_at).getTime()).filter((t) => !isNaN(t))
  if (times.length < buckets) return null
  const max = Math.max(...times)
  const min = Math.min(...times)
  const span = max - min || 1
  const out = new Array(buckets).fill(0)
  for (const t of times) {
    const i = Math.min(buckets - 1, Math.floor(((t - min) / span) * buckets))
    out[i]++
  }
  return out
}

function statusChip(status: string) {
  const map: Record<string, string> = {
    success: 'text-good border-good/40',
    failed: 'text-crit border-crit/40',
    running: 'text-accent dark:text-accent-bright border-accent/40',
    pending: 'text-warn border-warn/40',
    approval_wait: 'text-warn border-warn/40',
  }
  return map[status] ?? 'text-muted-foreground border-border'
}
function statusStripe(status: string) {
  if (status === 'success') return 'bg-good'
  if (status === 'failed') return 'bg-crit'
  if (status === 'running' || status === 'pending' || status === 'approval_wait') return 'bg-accent dark:bg-accent-bright'
  return 'bg-faint'
}

function greeting() {
  const h = new Date().getHours()
  if (h < 12) return 'Good morning'
  if (h < 17) return 'Good afternoon'
  return 'Good evening'
}

export default function DashboardPage() {
  const { user, workspace } = useAuthStore()

  const { data: agentsData, isLoading: agentsLoading } = useQuery({ queryKey: ['agents'], queryFn: () => agentsAPI.list() as Promise<{ data: unknown[] }> })
  const { data: providersData, isLoading: providersLoading } = useQuery({ queryKey: ['providers'], queryFn: () => providersAPI.list() as Promise<{ data: unknown[] }> })
  const { data: runsData, isLoading: runsLoading } = useQuery({ queryKey: ['runs'], queryFn: () => runsAPI.list() as Promise<{ data: Run[] }> })
  const { data: triggersData } = useQuery({ queryKey: ['webhook-triggers'], queryFn: () => webhookTriggersAPI.list() as Promise<{ data: WebhookTrigger[] }> })
  const { data: connectorsData } = useQuery({ queryKey: ['connectors'], queryFn: () => connectorsAPI.list() as Promise<{ data: Connector[] }> })
  const { data: channelsData } = useQuery({ queryKey: ['gateway-channels'], queryFn: () => gatewayAPI.listChannels() as Promise<{ data: unknown[] }> })
  const { data: workflowsData } = useQuery({ queryKey: ['workflows'], queryFn: () => workflowsAPI.list() as Promise<{ data: unknown[] }> })
  const { data: evalSuitesData } = useQuery({ queryKey: ['eval-suites'], queryFn: () => evalsAPI.listSuites() as Promise<{ data: unknown[] }> })

  const statsLoading = agentsLoading || providersLoading || runsLoading

  const agentCount = agentsData?.data?.length ?? 0
  const providerCount = providersData?.data?.length ?? 0
  const runs = useMemo(() => runsData?.data ?? [], [runsData])
  const activeRuns = runs.filter((r) => ['pending', 'running', 'approval_wait'].includes(r.status)).length
  const succeeded = runs.filter((r) => r.status === 'success').length
  const finished = runs.filter((r) => ['success', 'failed'].includes(r.status)).length
  const successRate = finished > 0 ? Math.round((succeeded / finished) * 100) : null
  const tokens = runs.reduce((sum, r) => sum + r.total_input_tokens + r.total_output_tokens, 0)
  const triggers = triggersData?.data ?? []
  const activeTriggers = triggers.filter((t) => t.is_active).length
  const connectors = connectorsData?.data ?? []
  const channelCount = channelsData?.data?.length ?? 0
  const workflowCount = workflowsData?.data?.length ?? 0
  const evalSuiteCount = evalSuitesData?.data?.length ?? 0

  const series = useMemo(() => runsSeries(runs), [runs])
  const recentRuns = runs.slice(0, 6)

  const secondary = [
    { label: 'Workflows', value: workflowCount, href: '/workflows', icon: GitBranch },
    { label: 'Triggers', value: `${activeTriggers}/${triggers.length}`, href: '/triggers', icon: Zap },
    { label: 'Connectors', value: connectors.length, href: '/connectors', icon: Database },
    { label: 'Channels', value: channelCount, href: '/gateway', icon: Plug },
    { label: 'Eval suites', value: evalSuiteCount, href: '/evals', icon: Activity },
  ]

  const checklist = [
    { key: 'provider', label: 'Configure a model provider', done: providerCount > 0, href: '/settings/providers' },
    { key: 'agent', label: 'Create your first agent', done: agentCount > 0, href: '/agents/new' },
    { key: 'connector', label: 'Connect a data source', done: connectors.length > 0, href: '/connectors' },
    { key: 'run', label: 'Run a conversation', done: runs.length > 0, href: '/playground' },
  ]
  const checklistDone = checklist.every((c) => c.done)

  const [dismissed, setDismissed] = useState(true)
  useEffect(() => {
    if (!workspace?.id) return
    setDismissed(localStorage.getItem(`onboarding-dismissed-${workspace.id}`) === 'true')
  }, [workspace?.id])
  const dismissChecklist = () => {
    if (workspace?.id) localStorage.setItem(`onboarding-dismissed-${workspace.id}`, 'true')
    setDismissed(true)
  }

  return (
    <div className="p-4 sm:p-6 max-w-6xl">
      <PageHeader
        eyebrow="Overview"
        title={`${greeting()}, ${user?.full_name?.split(' ')[0] ?? 'there'}`}
        subtitle={workspace?.display_name}
        actions={
          <Link href="/agents/new" className="inline-flex items-center gap-2 px-3.5 py-2 rounded-[10px] text-[13px] font-semibold text-white bg-gradient-to-br from-accent to-accent-ink shadow-card hover:opacity-95">
            <Bot className="w-4 h-4" /> New agent
          </Link>
        }
      />

      {/* Onboarding checklist */}
      {!statsLoading && !checklistDone && !dismissed && (
        <div className="border border-accent/25 bg-accent/[0.06] rounded-2xl p-4 sm:p-5 mb-6 relative">
          <button onClick={dismissChecklist} className="absolute top-3 right-3 text-muted-foreground hover:text-foreground" aria-label="Dismiss setup checklist">
            <X size={14} />
          </button>
          <h2 className="text-sm font-semibold text-foreground mb-3 pr-6">Get started with Agent Nexus</h2>
          <div className="flex flex-col gap-1.5">
            {checklist.map((c) => (
              <Link key={c.key} href={c.href} className={cn('flex items-center gap-2.5 text-sm rounded-lg px-2 py-1.5 -mx-2 transition-colors', c.done ? 'text-muted-foreground' : 'text-foreground hover:bg-surface')}>
                <span className={cn('w-4 h-4 rounded-full flex items-center justify-center flex-shrink-0 border', c.done ? 'bg-good border-good' : 'border-border-strong')}>
                  {c.done && <Check size={10} className="text-white" />}
                </span>
                <span className={c.done ? 'line-through' : ''}>{c.label}</span>
              </Link>
            ))}
          </div>
        </div>
      )}

      {/* KPI instrument row */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3.5 mb-4">
        {statsLoading ? (
          Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="rounded-xl border border-border bg-surface shadow-card px-4 pt-3.5 pb-3">
              <Skeleton className="h-3 w-16 mb-3" /><Skeleton className="h-7 w-14 mb-3" /><Skeleton className="h-8 w-full" />
            </div>
          ))
        ) : (
          <>
            <KpiTile label="Total agents" value={agentCount} href="/agents" tone="accent" />
            <KpiTile label="Runs" value={runs.length} series={series ?? undefined} tone="accent" href="/runs" />
            <KpiTile label="Success rate" value={successRate ?? '—'} unit={successRate != null ? '%' : undefined} tone="good" href="/runs" />
            <KpiTile label="Tokens used" value={formatTokens(tokens)} tone="warn" href="/usage" />
          </>
        )}
      </div>

      {/* Secondary counts */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3.5 mb-6">
        {secondary.map((s) => (
          <Link key={s.label} href={s.href} className="flex items-center gap-3 rounded-xl border border-border bg-surface px-3.5 py-3 shadow-card hover:border-border-strong transition-colors">
            <div className="w-8 h-8 rounded-lg bg-accent/10 dark:bg-accent-bright/10 text-accent dark:text-accent-bright flex items-center justify-center flex-shrink-0">
              <s.icon size={15} />
            </div>
            <div className="min-w-0">
              <div className="text-lg font-bold tabular-nums text-foreground leading-none">{s.value}</div>
              <div className="text-[11px] text-muted-foreground mt-1 truncate">{s.label}</div>
            </div>
          </Link>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-[1.5fr_1fr] gap-5">
        {/* Quick actions */}
        <div>
          <span className="eyebrow block mb-3">Quick actions</span>
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
            {quickActions.map((a) => (
              <Link key={a.label} href={a.href} className="flex flex-col gap-2.5 rounded-xl border border-border bg-surface p-4 shadow-card hover:border-border-strong hover:-translate-y-0.5 transition-all">
                <div className="w-9 h-9 rounded-lg bg-accent/10 dark:bg-accent-bright/10 text-accent dark:text-accent-bright flex items-center justify-center">
                  <a.icon size={17} />
                </div>
                <div>
                  <p className="text-[13px] font-semibold text-foreground">{a.label}</p>
                  <p className="text-[11px] text-muted-foreground mt-0.5">{a.desc}</p>
                </div>
              </Link>
            ))}
            {/* Nexus fast-track promo */}
            <Link href="/agents/new" className="flex flex-col gap-2.5 rounded-xl border border-accent/30 bg-accent/[0.06] p-4 shadow-card hover:-translate-y-0.5 transition-all">
              <div className="w-9 h-9 rounded-lg bg-accent text-white flex items-center justify-center">
                <Sparkles size={17} />
              </div>
              <div>
                <p className="text-[13px] font-semibold text-foreground">Describe an agent</p>
                <p className="text-[11px] text-muted-foreground mt-0.5">Let Nexus draft it for you</p>
              </div>
            </Link>
          </div>
        </div>

        {/* Live activity */}
        <div>
          <div className="flex items-center justify-between mb-3">
            <span className="eyebrow">Live activity</span>
            <Link href="/runs" className="font-mono text-[11px] text-accent dark:text-accent-bright hover:underline">runs & traces →</Link>
          </div>
          <div className="rounded-xl border border-border bg-surface shadow-card overflow-hidden">
            {recentRuns.length === 0 ? (
              <div className="px-4 py-10 text-center text-[13px] text-muted-foreground">No runs yet.</div>
            ) : recentRuns.map((run) => (
              <Link key={run.id} href={`/runs/${run.id}`} className="grid grid-cols-[3px_1fr_auto] gap-3 items-center px-3.5 py-2.5 border-b border-border last:border-b-0 hover:bg-muted transition-colors">
                <span className={cn('self-stretch w-[3px] rounded-full', statusStripe(run.status))} />
                <div className="min-w-0">
                  <div className="font-mono text-[12px] text-foreground truncate">{run.id.slice(0, 8)}</div>
                  <div className="font-mono text-[10px] text-faint mt-0.5">{relativeTime(run.started_at)}</div>
                </div>
                <span className={cn('font-mono text-[10px] px-2 py-0.5 rounded-full border whitespace-nowrap', statusChip(run.status))}>{run.status}</span>
              </Link>
            ))}
          </div>
        </div>
      </div>

      {/* Empty state */}
      {agentCount === 0 && dismissed && (
        <div className="border border-dashed border-border-strong rounded-2xl p-10 text-center mt-6 bg-surface">
          <div className="w-12 h-12 rounded-full bg-muted flex items-center justify-center mx-auto mb-4">
            <Bot size={22} className="text-muted-foreground" />
          </div>
          <p className="text-sm font-medium text-foreground mb-1">No agents yet</p>
          <p className="text-xs text-muted-foreground mb-4">Create your first agent to start building with AI</p>
          <Link href="/agents/new" className="inline-flex px-4 py-2 bg-accent text-white text-sm rounded-lg hover:bg-accent-hover">Create agent</Link>
        </div>
      )}
    </div>
  )
}
