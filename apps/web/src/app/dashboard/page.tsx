'use client'

import Link from 'next/link'
import { useQuery } from '@tanstack/react-query'
import { Bot, Database, Key, Plug } from 'lucide-react'
import { agentsAPI, providersAPI, runsAPI } from '@/lib/api'
import { useAuthStore } from '@/store/auth'
import { formatTokens } from '@/lib/utils'
import type { Run } from '@/types'

const quickActions = [
  { label: 'Create Agent', href: '/agents/new', icon: Bot, color: 'bg-purple-50 text-purple-700' },
  { label: 'Connect MCP', href: '/mcp-servers', icon: Plug, color: 'bg-blue-50 text-blue-700' },
  { label: 'Add Source', href: '/connectors', icon: Database, color: 'bg-teal-50 text-teal-700' },
  { label: 'Add API Key', href: '/settings/providers', icon: Key, color: 'bg-amber-50 text-amber-700' },
]

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

  const agentCount = agentsData?.data?.length ?? 0
  const providerCount = providersData?.data?.length ?? 0
  const runs = runsData?.data ?? []
  const activeRuns = runs.filter((run) => ['pending', 'running', 'approval_wait'].includes(run.status)).length
  const tokens = runs.reduce((sum, run) => sum + run.total_input_tokens + run.total_output_tokens, 0)

  const stats = [
    { label: 'Total Agents', value: String(agentCount), sub: 'in this workspace' },
    { label: 'Providers Connected', value: String(providerCount), sub: 'API keys configured' },
    { label: 'Active Runs', value: String(activeRuns), sub: `${runs.length} total runs` },
    { label: 'Tokens recorded', value: formatTokens(tokens), sub: 'across all runs' },
  ]

  return (
    <div className="p-6 max-w-6xl">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-gray-900">
          Welcome back, {user?.full_name?.split(' ')[0] ?? 'there'}
        </h1>
        <p className="text-sm text-gray-500 mt-0.5">{workspace?.display_name}</p>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-4 gap-4 mb-8">
        {stats.map((s) => (
          <div key={s.label} className="border border-gray-100 rounded-xl p-4 bg-white">
            <div className="text-2xl font-bold text-gray-900">{s.value}</div>
            <div className="text-sm font-medium text-gray-700 mt-1">{s.label}</div>
            <div className="text-xs text-gray-400 mt-0.5">{s.sub}</div>
          </div>
        ))}
      </div>

      {/* Quick Actions */}
      <div className="mb-8">
        <h2 className="text-sm font-medium text-gray-500 uppercase tracking-wide mb-3">Quick Actions</h2>
        <div className="grid grid-cols-4 gap-3">
          {quickActions.map((a) => (
            <Link
              key={a.label}
              href={a.href}
              className="flex items-center gap-3 border border-gray-100 rounded-xl p-4 hover:border-gray-200 hover:shadow-sm transition-all"
            >
              <div className={`w-9 h-9 rounded-lg ${a.color} flex items-center justify-center`}>
                <a.icon size={18} />
              </div>
              <span className="text-sm font-medium text-gray-700">{a.label}</span>
            </Link>
          ))}
        </div>
      </div>

      {/* Getting started */}
      {agentCount === 0 && (
        <div className="border border-dashed border-gray-200 rounded-xl p-8 text-center">
          <Bot size={32} className="mx-auto text-gray-300 mb-3" />
          <p className="text-gray-500 text-sm">No agents yet. Create your first agent to get started.</p>
          <Link href="/agents/new">
            <button className="mt-4 px-4 py-2 bg-purple-600 text-white text-sm rounded-lg hover:bg-purple-700">
              Create Agent
            </button>
          </Link>
        </div>
      )}
    </div>
  )
}
