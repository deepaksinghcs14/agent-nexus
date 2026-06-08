'use client'

import { useQuery } from '@tanstack/react-query'
import { Building2 } from 'lucide-react'
import { adminAPI } from '@/lib/api'
import { relativeTime } from '@/lib/utils'
import type { Workspace } from '@/types'

export default function AdminWorkspacesPage() {
  const { data, isLoading, error } = useQuery({ queryKey: ['admin-workspaces'], queryFn: () => adminAPI.workspaces() as Promise<{ data: Workspace[] }> })
  const workspaces = data?.data ?? []
  return <div className="p-6 max-w-4xl"><div className="mb-5"><h1 className="text-lg font-semibold text-gray-900">Workspaces</h1><p className="text-sm text-gray-500 mt-0.5">Manage all workspaces on this instance</p></div>{error && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">{(error as Error).message}</div>}{isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading…</div>}{!isLoading && !error && workspaces.length === 0 && <div className="border border-dashed border-gray-200 rounded-xl py-12 text-center"><Building2 className="mx-auto text-gray-300 mb-3" /><p className="text-sm text-gray-500">No workspaces found.</p></div>}<div className="bg-white border border-gray-100 rounded-xl overflow-hidden">{workspaces.map((workspace) => <div key={workspace.id} className="grid grid-cols-12 px-4 py-3 border-b last:border-b-0 border-gray-50 text-[12px]"><div className="col-span-5"><p className="font-medium text-gray-900">{workspace.display_name}</p><p className="text-[10px] text-gray-400">{workspace.name}</p></div><span className="col-span-4 text-gray-500 font-mono text-[10px]">{workspace.owner_id}</span><span className="col-span-3 text-gray-400">{relativeTime(workspace.created_at)}</span></div>)}</div></div>
}
