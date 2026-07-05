'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useQuery } from '@tanstack/react-query'
import { Activity, Bot, Check, Database, FlaskConical, GitBranch, Key, Plug, Radio, X, Zap, type LucideIcon } from 'lucide-react'
import { agentsAPI, connectorsAPI, evalsAPI, gatewayAPI, providersAPI, runsAPI, webhookTriggersAPI, workflowsAPI } from '@/lib/api'
import { useAuthStore } from '@/store/auth'
import { formatTokens, relativeTime, cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import type { Connector, Run, WebhookTrigger } from '@/types'

const quickActions = [
  { label: 'Create Agent', desc: 'Build a new AI agent', href: '/agents/new', icon: Bot, color: 'bg-purple-50 dark:bg-purple-500/10 text-purple-700 dark:text-purple-300' },
  { label: 'Connect MCP', desc: 'Add an MCP server', href: '/mcp-servers', icon: Plug, color: 'bg-blue-50 dark:bg-blue-500/10 text-blue-700 dark:text-blue-300' },
  { label: 'Add Source', desc: 'Connect a data source', href: '/connectors', icon: Database, color: 'bg-teal-50 dark:bg-teal-500/10 text-teal-700 dark:text-teal-300' },
  { label: 'Add API Key', desc: 'Configure a provider', href: '/settings/providers', icon: Key, color: 'bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300' },
  { label: 'New Trigger', desc: 'Wire a webhook event', href: '/triggers/new', icon: Zap, color: 'bg-rose-50 dark:bg-rose-500/10 text-rose-700 dark:text-rose-300' },
]

function runStatusDot(status: string) {
  if (status === 'success') return 'bg-green-400'
  if (status === 'failed') return 'bg-red-400'
  if (status === 'running' || status === 'pending') return 'bg-amber-400'
  return 'bg-gray-300'
}

function greeting() {
  const h = new Date().getHours()
  if (h < 12) return 'Good morning'
  if (h < 17) return 'Good afternoon'
  return 'Good evening'
}

interface StatCard {
  label: string
  value: string
  sub: string
  icon: LucideIcon
  color: string
  href: string
}

export default function DashboardPage() {
  const { user, workspace } = useAuthStore()

  const { data: agentsData, isLoading: agentsLoading } = useQuery({ queryKey: ['agents'], queryFn: () => agentsAPI.list() as Promise<{ data: unknown[] }> })
  const { data: providersData, isLoading: providersLoading } = useQuery({ queryKey: ['providers'], queryFn: () => providersAPI.list() as Promise<{ data: unknown[] }> })
  const { data: runsData, isLoading: runsLoading } = useQuery({ queryKey: ['runs'], queryFn: () => runsAPI.list() as Promise<{ data: Run[] }> })
  const { data: triggersData, isLoading: triggersLoading } = useQuery({ queryKey: ['webhook-triggers'], queryFn: () => webhookTriggersAPI.list() as Promise<{ data: WebhookTrigger[] }> })
  const { data: connectorsData, isLoading: connectorsLoading } = useQuery({ queryKey: ['connectors'], queryFn: () => connectorsAPI.list() as Promise<{ data: Connector[] }> })
  const { data: channelsData, isLoading: channelsLoading } = useQuery({ queryKey: ['gateway-channels'], queryFn: () => gatewayAPI.listChannels() as Promise<{ data: unknown[] }> })
  const { data: workflowsData, isLoading: workflowsLoading } = useQuery({ queryKey: ['workflows'], queryFn: () => workflowsAPI.list() as Promise<{ data: unknown[] }> })
  const { data: evalSuitesData, isLoading: evalSuitesLoading } = useQuery({ queryKey: ['eval-suites'], queryFn: () => evalsAPI.listSuites() as Promise<{ data: unknown[] }> })

  const statsLoading = agentsLoading || providersLoading || runsLoading || triggersLoading ||
    connectorsLoading || channelsLoading || workflowsLoading || evalSuitesLoading

  const agentCount = agentsData?.data?.length ?? 0
  const providerCount = providersData?.data?.length ?? 0
  const runs = runsData?.data ?? []
  const activeRuns = runs.filter((r) => ['pending', 'running', 'approval_wait'].includes(r.status)).length
  const tokens = runs.reduce((sum, r) => sum + r.total_input_tokens + r.total_output_tokens, 0)
  const triggers = triggersData?.data ?? []
  const activeTriggers = triggers.filter((t) => t.is_active).length
  const connectors = connectorsData?.data ?? []
  const syncingConnectors = connectors.filter((c) => (c as { sync_status?: string }).sync_status === 'syncing').length
  const channelCount = channelsData?.data?.length ?? 0
  const workflowCount = workflowsData?.data?.length ?? 0
  const evalSuiteCount = evalSuitesData?.data?.length ?? 0

  const stats: StatCard[] = [
    { label: 'Total Agents', value: String(agentCount), sub: 'in this workspace', icon: Bot, color: 'bg-purple-50 dark:bg-purple-500/10 text-purple-600 dark:text-purple-300', href: '/agents' },
    { label: 'Providers', value: String(providerCount), sub: 'API keys configured', icon: Key, color: 'bg-blue-50 dark:bg-blue-500/10 text-blue-600 dark:text-blue-300', href: '/settings/providers' },
    { label: 'Active Runs', value: String(activeRuns), sub: `${runs.length} total runs`, icon: Activity, color: 'bg-indigo-50 dark:bg-indigo-500/10 text-indigo-600 dark:text-indigo-300', href: '/runs' },
    { label: 'Tokens Used', value: formatTokens(tokens), sub: 'across all runs', icon: Zap, color: 'bg-green-50 dark:bg-green-500/10 text-green-600 dark:text-green-300', href: '/runs' },
    { label: 'Workflows', value: String(workflowCount), sub: 'automation pipelines', icon: GitBranch, color: 'bg-cyan-50 dark:bg-cyan-500/10 text-cyan-600 dark:text-cyan-300', href: '/workflows' },
    { label: 'Active Triggers', value: String(activeTriggers), sub: `${triggers.length} total configured`, icon: Zap, color: 'bg-amber-50 dark:bg-amber-500/10 text-amber-600 dark:text-amber-300', href: '/triggers' },
    { label: 'Connectors', value: String(connectors.length), sub: syncingConnectors > 0 ? `${syncingConnectors} syncing` : 'data sources', icon: Database, color: 'bg-teal-50 dark:bg-teal-500/10 text-teal-600 dark:text-teal-300', href: '/connectors' },
    { label: 'Gateway Channels', value: String(channelCount), sub: 'messaging channels', icon: Radio, color: 'bg-rose-50 dark:bg-rose-500/10 text-rose-600 dark:text-rose-300', href: '/gateway' },
    { label: 'Eval Suites', value: String(evalSuiteCount), sub: 'test suites', icon: FlaskConical, color: 'bg-fuchsia-50 dark:bg-fuchsia-500/10 text-fuchsia-600 dark:text-fuchsia-300', href: '/evals' },
  ]

  const recentRuns = runs.slice(0, 6)

  const checklist = [
    { key: 'provider',  label: 'Configure a model provider', done: providerCount > 0, href: '/settings/providers' },
    { key: 'agent',     label: 'Create your first agent',    done: agentCount > 0,    href: '/agents/new' },
    { key: 'connector', label: 'Connect a data source',      done: connectors.length > 0, href: '/connectors' },
    { key: 'run',       label: 'Run a conversation',         done: runs.length > 0,   href: '/playground' },
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
      {/* Hero greeting */}
      <div className="mb-8">
        <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">
          {greeting()}, {user?.full_name?.split(' ')[0] ?? 'there'}
        </h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">{workspace?.display_name}</p>
      </div>

      {/* Onboarding checklist — shown until every step is done or dismissed */}
      {!statsLoading && !checklistDone && !dismissed && (
        <div className="border border-purple-100 bg-purple-50 dark:bg-purple-500/10 rounded-xl p-4 sm:p-5 mb-8 relative">
          <button
            onClick={dismissChecklist}
            className="absolute top-3 right-3 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300"
            aria-label="Dismiss setup checklist"
          >
            <X size={14} />
          </button>
          <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100 mb-3 pr-6">Get started with Agent Nexus</h2>
          <div className="flex flex-col gap-2">
            {checklist.map((c) => (
              <Link
                key={c.key}
                href={c.href}
                className={cn(
                  'flex items-center gap-2.5 text-sm rounded-lg px-2 py-1.5 -mx-2 transition-colors',
                  c.done ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-300 hover:bg-white/70 dark:hover:bg-gray-800/70'
                )}
              >
                <span className={cn(
                  'w-4 h-4 rounded-full flex items-center justify-center flex-shrink-0 border',
                  c.done ? 'bg-green-500 border-green-500' : 'border-gray-300 dark:border-gray-600'
                )}>
                  {c.done && <Check size={10} className="text-white" />}
                </span>
                <span className={c.done ? 'line-through' : ''}>{c.label}</span>
              </Link>
            ))}
          </div>
        </div>
      )}

      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-8">
        {statsLoading
          ? Array.from({ length: 9 }).map((_, i) => (
              <div key={i} className="border border-gray-100 dark:border-gray-800 rounded-xl p-4 bg-white dark:bg-gray-900 flex items-start gap-3">
                <Skeleton className="w-9 h-9 rounded-lg flex-shrink-0" />
                <div className="flex-1">
                  <Skeleton className="h-6 w-12 mb-2" />
                  <Skeleton className="h-3.5 w-24 mb-1.5" />
                  <Skeleton className="h-3 w-20" />
                </div>
              </div>
            ))
          : stats.map((s) => (
              <Link key={s.label} href={s.href} className="border border-gray-100 dark:border-gray-800 rounded-xl p-4 bg-white dark:bg-gray-900 flex items-start gap-3 hover:border-purple-200 hover:shadow-sm transition-all">
                <div className={`w-9 h-9 rounded-lg ${s.color} flex items-center justify-center flex-shrink-0`}>
                  <s.icon size={17} />
                </div>
                <div>
                  <div className="text-2xl font-bold text-gray-900 dark:text-gray-100 leading-tight">{s.value}</div>
                  <div className="text-sm font-medium text-gray-700 dark:text-gray-300 mt-0.5">{s.label}</div>
                  <div className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">{s.sub}</div>
                </div>
              </Link>
            ))}
      </div>

      {/* Quick Actions */}
      <div className="mb-8">
        <h2 className="text-xs font-medium text-gray-400 dark:text-gray-500 uppercase tracking-wider mb-3">Quick Actions</h2>
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-5 gap-3">
          {quickActions.map((a) => (
            <Link
              key={a.label}
              href={a.href}
              className="flex flex-col gap-2 border border-gray-100 dark:border-gray-800 rounded-xl p-4 hover:border-purple-200 hover:shadow-sm transition-all"
            >
              <div className={`w-9 h-9 rounded-lg ${a.color} flex items-center justify-center`}>
                <a.icon size={18} />
              </div>
              <div>
                <p className="text-sm font-medium text-gray-800 dark:text-gray-200">{a.label}</p>
                <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">{a.desc}</p>
              </div>
            </Link>
          ))}
        </div>
      </div>

      {/* Recent runs */}
      {recentRuns.length > 0 && (
        <div className="mb-8">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-xs font-medium text-gray-400 dark:text-gray-500 uppercase tracking-wider">Recent Runs</h2>
            <Link href="/runs" className="text-xs text-purple-600 dark:text-purple-300 hover:underline">View all</Link>
          </div>
          <div className="bg-white dark:bg-gray-900 border border-gray-100 dark:border-gray-800 rounded-xl divide-y divide-gray-50 dark:divide-gray-800">
            {recentRuns.map((run) => (
              <Link
                key={run.id}
                href={`/runs/${run.id}`}
                className="flex items-center gap-2.5 px-4 py-3 hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors"
              >
                <span className={`w-2 h-2 rounded-full flex-shrink-0 ${runStatusDot(run.status)}`} />
                <span className="text-sm text-gray-700 dark:text-gray-300 flex-1 truncate font-mono text-xs">{run.id.slice(0, 8)}</span>
                <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${
                  run.status === 'success' ? 'bg-green-50 dark:bg-green-500/10 text-green-700 dark:text-green-300' :
                  run.status === 'failed' ? 'bg-red-50 dark:bg-red-500/10 text-red-700 dark:text-red-300' :
                  run.status === 'running' ? 'bg-blue-50 dark:bg-blue-500/10 text-blue-700 dark:text-blue-300' :
                  'bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-400'
                }`}>{run.status}</span>
                <span className="text-xs text-gray-400 dark:text-gray-500 ml-auto">{relativeTime(run.started_at)}</span>
              </Link>
            ))}
          </div>
        </div>
      )}

      {/* Fallback empty state — only when the checklist above has been dismissed */}
      {agentCount === 0 && dismissed && (
        <div className="border border-dashed border-gray-200 dark:border-gray-700 rounded-xl p-10 text-center">
          <div className="w-12 h-12 rounded-full bg-gray-100 dark:bg-gray-800 flex items-center justify-center mx-auto mb-4">
            <Bot size={22} className="text-gray-400 dark:text-gray-500" />
          </div>
          <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">No agents yet</p>
          <p className="text-xs text-gray-400 dark:text-gray-500 mb-4">Create your first agent to start building with AI</p>
          <Link href="/agents/new">
            <button className="px-4 py-2 bg-purple-600 text-white text-sm rounded-lg hover:bg-purple-700">
              Create Agent
            </button>
          </Link>
        </div>
      )}
    </div>
  )
}
