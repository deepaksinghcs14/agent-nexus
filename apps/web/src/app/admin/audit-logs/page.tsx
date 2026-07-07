'use client'

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ClipboardList } from 'lucide-react'
import { adminAPI } from '@/lib/api'
import { relativeTime } from '@/lib/utils'
import type { AuditLog } from '@/types'

export default function AdminAuditLogsPage() {
  const [resourceType, setResourceType] = useState('')
  const [actorEmail, setActorEmail] = useState('')

  const params = [
    resourceType ? `resource_type=${encodeURIComponent(resourceType)}` : '',
    actorEmail ? `actor_email=${encodeURIComponent(actorEmail)}` : '',
  ].filter(Boolean).join('&')

  const { data, isLoading, error } = useQuery({
    queryKey: ['admin-audit-logs', resourceType, actorEmail],
    queryFn: () => adminAPI.auditLogs(params) as Promise<{ data: AuditLog[] }>,
  })
  const logs = data?.data ?? []

  return (
    <div className="p-4 sm:p-6">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-4">
        <h1 className="text-xl font-semibold text-foreground">Audit logs</h1>
        <div className="flex flex-wrap gap-2">
          <input
            value={actorEmail}
            onChange={(e) => setActorEmail(e.target.value)}
            placeholder="Filter by actor email"
            className="text-[12px] px-3 py-1.5 border border-border-strong rounded-lg w-44"
          />
          <input
            value={resourceType}
            onChange={(e) => setResourceType(e.target.value)}
            placeholder="Filter resource type"
            className="text-[12px] px-3 py-1.5 border border-border-strong rounded-lg w-40"
          />
        </div>
      </div>
      {error && (
        <div className="text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg p-3 mb-4">{(error as Error).message}</div>
      )}
      {isLoading && <div className="py-12 text-center text-sm text-faint">Loading…</div>}
      {!isLoading && !error && logs.length === 0 && (
        <div className="border border-dashed border-border-strong rounded-xl py-12 text-center">
          <ClipboardList className="mx-auto text-faint mb-3" />
          <p className="text-sm text-muted-foreground">No audit events found.</p>
        </div>
      )}
      <div className="overflow-x-auto -mx-4 sm:mx-0">
        <div className="min-w-[600px] px-4 sm:px-0">
          <div className="bg-surface border border-border rounded-xl overflow-hidden">
            {logs.map((log) => (
              <div key={log.id} className="grid grid-cols-12 gap-3 px-4 py-3 border-b last:border-b-0 border-gray-50">
                <div className="col-span-3">
                  <p className="text-[12px] font-medium text-foreground">{log.actor_email || 'system'}</p>
                  <p className="text-[10px] text-faint">{log.ip_address}</p>
                </div>
                <div className="col-span-5 text-[12px] text-muted-foreground">{log.action}</div>
                <div className="col-span-2 text-[11px] text-faint">{log.resource_type}</div>
                <div className="col-span-2 text-[11px] text-faint">{relativeTime(log.created_at)}</div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
