'use client'

import Link from 'next/link'
import { useQuery } from '@tanstack/react-query'
import { Activity, Bot, Database, Key, Plug, Zap } from 'lucide-react'
import { agentsAPI, providersAPI, runsAPI, webhookTriggersAPI } from '@/lib/api'
import { useAuthStore } from '@/store/auth'
import { formatTokens, relativeTime } from '@/lib/utils'
import type { Run, WebhookTrigger } from '@/types'

const quickActions = [
  { label: 'Create Agent', desc: 'Build a new AI agent', href: '/agents/new', icon: Bot, color: 'bg-purple-50 text-purple-700' },
  { label: 'Connect MCP', desc: 'Add an MCP server', href: '/mcp-servers', icon: Plug, color: 'bg-blue-50 text-blue-700' },
  { label: 'Add Source', desc: 'Connect a data source', href: '/connectors', icon: Database, color: 'bg-teal-50 text-teal-700' },
  { label: 'Add API Key', desc: 'Configure a provider', href: '/settings/providers', icon: Key, color: 'bg-amber-50 text-amber-700' },
  { label: 'New Trigger', desc: 'Wire a webhook event', href: '/triggers/new', icon: Zap, color: 'bg-rose-50 text-rose-700' },
]

const statIcons = [
  { icon: Bot, color: 'bg-purple-50 text-purple-600' },
  { icon: Key, color: 'bg-blue-50 text-blue-600' },
  { icon: Activity, color: 'bg-indigo-50 text-indigo-600' },
  { icon: Zap, color: 'bg-green-50 text-green-600' },
  { icon: Plug, color: 'bg-amber-50 text-amber-600' },
  { icon: Database, color: 'bg-rose-50 text-rose-600' },
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

export default function DashboardPage() {
  const { user, workspace } = useAuthStore()

  const { data: agentsData } = useQuery({
    queryKey: ['agents'],
    queryFn: () => agentsAPI.list() as Promise<{ data: unknown[] }>,
  })
  const { data: providersData } = useQuery({
    queryKey: ['providers'],
    queryFn: () => providersAPI.list() as Promise<{ data: unknown[] }>,
  })
  const { data: runsData } = useQuery({
    queryKey: ['runs'],
    queryFn: () => runsAPI.list() as Promise<{ data: Run[] }>,
  })
  const { data: triggersData } = useQuery({
    queryKey: ['webhook-triggers'],
    queryFn: () => webhookTriggersAPI.list() as Promise<{ data: WebhookTrigger[] }>,
  })

  const agentCount = agentsData?.data?.length ?? 0
  const providerCount = providersData?.data?.length ?? 0
  const runs = runsData?.data ?? []
  const activeRuns = runs.filter((r) => ['pending', 'running', 'approval_wait'].includes(r.status)).length
  const tokens = runs.reduce((sum, r) => sum + r.total_input_tokens + r.total_output_tokens, 0)
  const triggers = triggersData?.data ?? []
  const activeTriggers = triggers.filter((t) => t.is_active).length
  const totalFired = triggers.reduce((sum, t) => sum + t.trigger_count, 0)

  const stats = [
    { label: 'Total Agents', value: String(agentCount), sub: 'in this workspace' },
    { label: 'Providers Connected', value: String(providerCount), sub: 'API keys configured' },
    { label: 'Active Runs', value: String(activeRuns), sub: `${runs.length} total runs` },
    { label: 'Tokens Recorded', value: formatTokens(tokens), sub: 'across all runs' },
    { label: 'Active Triggers', value: String(activeTriggers), sub: `${triggers.length} total configured` },
    { label: 'Times Fired', value: String(totalFired), sub: 'webhook invocations' },
  ]

  const recentRuns = runs.slice(0, 6)

  return (
    <div className="p-4 sm:p-6 max-w-6xl">
      {/* Hero greeting */}
      <div className="mb-8">
        <h1 className="text-xl font-semibold text-gray-900">
          {greeting()}, {user?.full_name?.split(' ')[0] ?? 'there'}
        </h1>
        <p className="text-sm text-gray-500 mt-0.5">{workspace?.display_name}</p>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-8">
        {stats.map((s, i) => {
          const { icon: Icon, color } = statIcons[i]
          return (
            <div key={s.label} className="border border-gray-100 rounded-xl p-4 bg-white flex items-start gap-3">
              <div className={`w-9 h-9 rounded-lg ${color} flex items-center justify-center flex-shrink-0`}>
                <Icon size={17} />
              </div>
              <div>
                <div className="text-2xl font-bold text-gray-900 leading-tight">{s.value}</div>
                <div className="text-sm font-medium text-gray-700 mt-0.5">{s.label}</div>
                <div className="text-xs text-gray-400 mt-0.5">{s.sub}</div>
              </div>
            </div>
          )
        })}
      </div>

      {/* Quick Actions */}
      <div className="mb-8">
        <h2 className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-3">Quick Actions</h2>
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-5 gap-3">
          {quickActions.map((a) => (
            <Link
              key={a.label}
              href={a.href}
              className="flex flex-col gap-2 border border-gray-100 rounded-xl p-4 hover:border-purple-200 hover:shadow-sm transition-all"
            >
              <div className={`w-9 h-9 rounded-lg ${a.color} flex items-center justify-center`}>
                <a.icon size={18} />
              </div>
              <div>
                <p className="text-sm font-medium text-gray-800">{a.label}</p>
                <p className="text-xs text-gray-400 mt-0.5">{a.desc}</p>
              </div>
            </Link>
          ))}
        </div>
      </div>

      {/* Recent runs */}
      {recentRuns.length > 0 && (
        <div className="mb-8">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-xs font-medium text-gray-400 uppercase tracking-wider">Recent Runs</h2>
            <Link href="/runs" className="text-xs text-purple-600 hover:underline">View all</Link>
          </div>
          <div className="bg-white border border-gray-100 rounded-xl divide-y divide-gray-50">
            {recentRuns.map((run) => (
              <Link
                key={run.id}
                href={`/runs/${run.id}`}
                className="flex items-center gap-2.5 px-4 py-3 hover:bg-gray-50/50 transition-colors"
              >
                <span className={`w-2 h-2 rounded-full flex-shrink-0 ${runStatusDot(run.status)}`} />
                <span className="text-sm text-gray-700 flex-1 truncate font-mono text-xs">{run.id.slice(0, 8)}</span>
                <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${
                  run.status === 'success' ? 'bg-green-50 text-green-700' :
                  run.status === 'failed' ? 'bg-red-50 text-red-700' :
                  run.status === 'running' ? 'bg-blue-50 text-blue-700' :
                  'bg-gray-100 text-gray-600'
                }`}>{run.status}</span>
                <span className="text-xs text-gray-400 ml-auto">{relativeTime(run.started_at)}</span>
              </Link>
            ))}
          </div>
        </div>
      )}

      {/* Empty state for new workspaces */}
      {agentCount === 0 && (
        <div className="border border-dashed border-gray-200 rounded-xl p-10 text-center">
          <div className="w-12 h-12 rounded-full bg-gray-100 flex items-center justify-center mx-auto mb-4">
            <Bot size={22} className="text-gray-400" />
          </div>
          <p className="text-sm font-medium text-gray-700 mb-1">No agents yet</p>
          <p className="text-xs text-gray-400 mb-4">Create your first agent to start building with AI</p>
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
