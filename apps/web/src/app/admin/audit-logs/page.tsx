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
        <h1 className="text-xl font-semibold text-gray-900">Audit logs</h1>
        <div className="flex flex-wrap gap-2">
          <input
            value={actorEmail}
            onChange={(e) => setActorEmail(e.target.value)}
            placeholder="Filter by actor email"
            className="text-[12px] px-3 py-1.5 border border-gray-200 rounded-lg w-44"
          />
          <input
            value={resourceType}
            onChange={(e) => setResourceType(e.target.value)}
            placeholder="Filter resource type"
            className="text-[12px] px-3 py-1.5 border border-gray-200 rounded-lg w-40"
          />
        </div>
      </div>
      {error && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">{(error as Error).message}</div>
      )}
      {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading…</div>}
      {!isLoading && !error && logs.length === 0 && (
        <div className="border border-dashed border-gray-200 rounded-xl py-12 text-center">
          <ClipboardList className="mx-auto text-gray-300 mb-3" />
          <p className="text-sm text-gray-500">No audit events found.</p>
        </div>
      )}
      <div className="overflow-x-auto -mx-4 sm:mx-0">
        <div className="min-w-[600px] px-4 sm:px-0">
          <div className="bg-white border border-gray-100 rounded-xl overflow-hidden">
            {logs.map((log) => (
              <div key={log.id} className="grid grid-cols-12 gap-3 px-4 py-3 border-b last:border-b-0 border-gray-50">
                <div className="col-span-3">
                  <p className="text-[12px] font-medium text-gray-900">{log.actor_email || 'system'}</p>
                  <p className="text-[10px] text-gray-400">{log.ip_address}</p>
                </div>
                <div className="col-span-5 text-[12px] text-gray-600">{log.action}</div>
                <div className="col-span-2 text-[11px] text-gray-400">{log.resource_type}</div>
                <div className="col-span-2 text-[11px] text-gray-400">{relativeTime(log.created_at)}</div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
