'use client'

import Link from 'next/link'
import { useQuery } from '@tanstack/react-query'
import { Activity, Building2, ClipboardList, Users, Zap, Coins, BarChart2, SquareTerminal, Bot, Database, Radio, FlaskConical } from 'lucide-react'
import { adminAPI } from '@/lib/api'
import { relativeTime, formatTokens } from '@/lib/utils'
import type { AuditLog, User } from '@/types'

interface WorkspaceUsage { id: string; display_name: string; runs: number; tokens: number; cost: number }
interface UsageData {
  runs: number; tokens: number; cost: number; webhook_triggers: number
  total_workspaces: number; total_agents: number; total_connectors: number
  total_gateway_channels: number; total_eval_suites: number
  top_workspaces: WorkspaceUsage[]
}

export default function AdminOverviewPage() {
  const users = useQuery({ queryKey: ['admin-users'], queryFn: () => adminAPI.users() as Promise<{ data: User[] }> })
  const audit = useQuery({ queryKey: ['admin-audit-logs', 'overview'], queryFn: () => adminAPI.auditLogs('limit=10') as Promise<{ data: AuditLog[] }> })
  const usage = useQuery({ queryKey: ['admin-usage'], queryFn: () => adminAPI.usage() as Promise<UsageData> })

  const logs = audit.data?.data ?? []
  const u = usage.data
  const errors = [users.error, audit.error, usage.error].filter(Boolean) as Error[]

  return (
    <div className="p-4 sm:p-6 max-w-5xl space-y-6">
      <div>
        <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">Admin Overview</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">Platform-wide stats and recent activity</p>
      </div>

      {errors.length > 0 && (
        <div className="text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg p-3">
          {errors[0].message}
        </div>
      )}

      {/* Platform entity counts */}
      <div>
        <p className="text-[11px] font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-3">Platform</p>
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
          {[
            { label: 'Users', value: users.data?.data?.length ?? '—', icon: Users, href: '/admin/users' },
            { label: 'Workspaces', value: u?.total_workspaces ?? '—', icon: Building2, href: '/admin/workspaces' },
            { label: 'Agents', value: u?.total_agents ?? '—', icon: Bot, href: '/admin/workspaces' },
            { label: 'Connectors', value: u?.total_connectors ?? '—', icon: Database, href: '/admin/workspaces' },
            { label: 'Gateway channels', value: u?.total_gateway_channels ?? '—', icon: Radio, href: '/admin/workspaces' },
            { label: 'Eval suites', value: u?.total_eval_suites ?? '—', icon: FlaskConical, href: '/admin/workspaces' },
            { label: 'Webhook triggers', value: u?.webhook_triggers ?? '—', icon: Zap, href: '/triggers' },
            { label: 'Live console', value: 'Stream', icon: SquareTerminal, href: '/admin/service-logs' },
            { label: 'Audit events', value: logs.length > 0 ? `${logs.length}+` : (audit.isLoading ? '—' : '0'), icon: ClipboardList, href: '/admin/audit-logs' },
          ].map((item) => (
            <Link key={item.label} href={item.href}
              className="bg-gray-50 dark:bg-gray-800/60 border border-gray-100 dark:border-gray-800 rounded-xl p-4 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors">
              <item.icon className="w-4 h-4 text-purple-600 dark:text-purple-300 mb-3" />
              <p className="text-2xl font-semibold text-gray-900 dark:text-gray-100">{item.value}</p>
              <p className="text-[11px] text-gray-400 dark:text-gray-500 mt-1">{item.label}</p>
            </Link>
          ))}
        </div>
      </div>

      {/* Usage stats */}
      <div>
        <p className="text-[11px] font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider mb-3">Usage (all time)</p>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          {[
            { label: 'Agent runs', value: u ? u.runs.toLocaleString() : '—', icon: Zap, color: 'text-amber-500' },
            { label: 'Tokens used', value: u ? formatTokens(u.tokens) : '—', icon: BarChart2, color: 'text-blue-500' },
            { label: 'Est. cost', value: u ? `$${u.cost.toFixed(4)}` : '—', icon: Coins, color: 'text-green-600 dark:text-green-300' },
          ].map((item) => (
            <div key={item.label} className="bg-gray-50 dark:bg-gray-800/60 border border-gray-100 dark:border-gray-800 rounded-xl p-4">
              <item.icon className={`w-4 h-4 mb-3 ${item.color}`} />
              <p className="text-2xl font-semibold text-gray-900 dark:text-gray-100">{item.value}</p>
              <p className="text-[11px] text-gray-400 dark:text-gray-500 mt-1">{item.label}</p>
            </div>
          ))}
        </div>
      </div>

      {/* Top workspaces by usage */}
      {u?.top_workspaces && u.top_workspaces.length > 0 && (
        <div>
          <div className="flex items-center justify-between mb-2">
            <p className="text-[11px] font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Top workspaces by runs</p>
            <Link href="/admin/workspaces" className="text-[11px] text-purple-600 dark:text-purple-300 hover:underline">View all</Link>
          </div>
          <div className="bg-white dark:bg-gray-900 border border-gray-100 dark:border-gray-800 rounded-xl overflow-hidden">
            {u.top_workspaces.map((ws, i) => (
              <div key={ws.id} className="flex items-center gap-3 px-4 py-3 border-b last:border-b-0 border-gray-50">
                <span className="text-[11px] font-semibold text-gray-300 dark:text-gray-600 w-4">{i + 1}</span>
                <div className="flex-1 min-w-0">
                  <p className="text-[12px] font-medium text-gray-900 dark:text-gray-100 truncate">{ws.display_name}</p>
                  <p className="text-[10px] text-gray-400 dark:text-gray-500">{formatTokens(ws.tokens)} tokens · ${ws.cost.toFixed(4)}</p>
                </div>
                <span className="text-[12px] font-semibold text-gray-700 dark:text-gray-300">{ws.runs.toLocaleString()} runs</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Recent audit events */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <p className="text-[11px] font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Recent activity</p>
          <Link href="/admin/audit-logs" className="text-[11px] text-purple-600 dark:text-purple-300 hover:underline">View all</Link>
        </div>
        <div className="bg-white dark:bg-gray-900 border border-gray-100 dark:border-gray-800 rounded-xl overflow-hidden">
          {audit.isLoading && (
            <div className="p-6 text-center text-[12px] text-gray-400 dark:text-gray-500">Loading…</div>
          )}
          {logs.map((log) => (
            <div key={log.id} className="flex gap-3 px-4 py-3 border-b last:border-b-0 border-gray-50">
              <Activity className="w-3.5 h-3.5 text-purple-500 mt-0.5 flex-shrink-0" />
              <div className="flex-1 min-w-0">
                <p className="text-[12px] text-gray-700 dark:text-gray-300 truncate">
                  <span className="font-medium text-gray-900 dark:text-gray-100">{log.actor_email || 'system'}</span>
                  {' · '}{log.action}
                </p>
                <p className="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">{log.resource_type}{log.ip_address ? ` · ${log.ip_address}` : ''}</p>
              </div>
              <span className="text-[10px] text-gray-400 dark:text-gray-500 flex-shrink-0">{relativeTime(log.created_at)}</span>
            </div>
          ))}
          {!audit.isLoading && logs.length === 0 && (
            <div className="p-8 text-center text-[12px] text-gray-400 dark:text-gray-500">
              No activity recorded yet. Events appear here when users log in or admins make changes.
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
