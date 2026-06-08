'use client'

import Link from 'next/link'
import { useQuery } from '@tanstack/react-query'
import { Activity, Building2, ClipboardList, Users, Zap, Coins, BarChart2 } from 'lucide-react'
import { adminAPI } from '@/lib/api'
import { relativeTime } from '@/lib/utils'
import type { AuditLog, User, Workspace } from '@/types'

interface UsageData { runs: number; tokens: number; cost: number }

export default function AdminOverviewPage() {
  const users = useQuery({ queryKey: ['admin-users'], queryFn: () => adminAPI.users() as Promise<{ data: User[] }> })
  const workspaces = useQuery({ queryKey: ['admin-workspaces'], queryFn: () => adminAPI.workspaces() as Promise<{ data: Workspace[] }> })
  const audit = useQuery({ queryKey: ['admin-audit-logs', 'overview'], queryFn: () => adminAPI.auditLogs('limit=10') as Promise<{ data: AuditLog[] }> })
  const usage = useQuery({ queryKey: ['admin-usage'], queryFn: () => adminAPI.usage() as Promise<UsageData> })

  const logs = audit.data?.data ?? []
  const u = usage.data
  const errors = [users.error, workspaces.error, audit.error, usage.error].filter(Boolean) as Error[]

  return (
    <div className="p-6 max-w-5xl space-y-6">
      {errors.length > 0 && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3">
          {errors[0].message}
        </div>
      )}

      {/* Platform stats */}
      <div>
        <p className="text-[11px] font-medium text-gray-500 uppercase tracking-wider mb-3">Platform</p>
        <div className="grid grid-cols-3 gap-3">
          {[
            { label: 'Users', value: users.data?.data?.length ?? '—', icon: Users, href: '/admin/users' },
            { label: 'Workspaces', value: workspaces.data?.data?.length ?? '—', icon: Building2, href: '/admin/workspaces' },
            { label: 'Audit events', value: logs.length > 0 ? `${logs.length}+` : (audit.isLoading ? '—' : '0'), icon: ClipboardList, href: '/admin/audit-logs' },
          ].map((item) => (
            <Link key={item.label} href={item.href}
              className="bg-gray-50 border border-gray-100 rounded-xl p-4 hover:bg-gray-100 transition-colors">
              <item.icon className="w-4 h-4 text-purple-600 mb-3" />
              <p className="text-2xl font-semibold text-gray-900">{item.value}</p>
              <p className="text-[11px] text-gray-400 mt-1">{item.label}</p>
            </Link>
          ))}
        </div>
      </div>

      {/* Usage stats */}
      <div>
        <p className="text-[11px] font-medium text-gray-500 uppercase tracking-wider mb-3">Usage (all time)</p>
        <div className="grid grid-cols-3 gap-3">
          {[
            { label: 'Agent runs', value: u ? u.runs.toLocaleString() : '—', icon: Zap, color: 'text-amber-500' },
            { label: 'Tokens used', value: u ? u.tokens.toLocaleString() : '—', icon: BarChart2, color: 'text-blue-500' },
            { label: 'Est. cost', value: u ? `$${u.cost.toFixed(4)}` : '—', icon: Coins, color: 'text-green-600' },
          ].map((item) => (
            <div key={item.label} className="bg-gray-50 border border-gray-100 rounded-xl p-4">
              <item.icon className={`w-4 h-4 mb-3 ${item.color}`} />
              <p className="text-2xl font-semibold text-gray-900">{item.value}</p>
              <p className="text-[11px] text-gray-400 mt-1">{item.label}</p>
            </div>
          ))}
        </div>
      </div>

      {/* Recent audit events */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <p className="text-[11px] font-medium text-gray-500 uppercase tracking-wider">Recent activity</p>
          <Link href="/admin/audit-logs" className="text-[11px] text-purple-600 hover:underline">View all</Link>
        </div>
        <div className="bg-white border border-gray-100 rounded-xl overflow-hidden">
          {audit.isLoading && (
            <div className="p-6 text-center text-[12px] text-gray-400">Loading…</div>
          )}
          {logs.map((log) => (
            <div key={log.id} className="flex gap-3 px-4 py-3 border-b last:border-b-0 border-gray-50">
              <Activity className="w-3.5 h-3.5 text-purple-500 mt-0.5 flex-shrink-0" />
              <div className="flex-1 min-w-0">
                <p className="text-[12px] text-gray-700 truncate">
                  <span className="font-medium text-gray-900">{log.actor_email || 'system'}</span>
                  {' · '}{log.action}
                </p>
                <p className="text-[10px] text-gray-400 mt-0.5">{log.resource_type}{log.ip_address ? ` · ${log.ip_address}` : ''}</p>
              </div>
              <span className="text-[10px] text-gray-400 flex-shrink-0">{relativeTime(log.created_at)}</span>
            </div>
          ))}
          {!audit.isLoading && logs.length === 0 && (
            <div className="p-8 text-center text-[12px] text-gray-400">
              No activity recorded yet. Events appear here when users log in or admins make changes.
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
