'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Building2, Users, Briefcase, FlaskConical, User } from 'lucide-react'
import { workspacesAPI } from '@/lib/api'
import { useAuthStore } from '@/store/auth'
import { cn } from '@/lib/utils'
import type { WorkspaceType } from '@/types'

const TYPES: { value: WorkspaceType; label: string; description: string; icon: React.ElementType }[] = [
  { value: 'personal', label: 'Personal', description: 'Just for you — side projects and experiments', icon: User },
  { value: 'team', label: 'Team', description: 'A shared space for a small group', icon: Users },
  { value: 'organization', label: 'Organization', description: 'Company-wide agents and workflows', icon: Building2 },
  { value: 'project', label: 'Project', description: 'Scoped to a specific product or initiative', icon: Briefcase },
  { value: 'sandbox', label: 'Sandbox', description: 'Testing and prototyping — anything goes', icon: FlaskConical },
]

export default function NewWorkspacePage() {
  const router = useRouter()
  const workspaces = useAuthStore((s) => s.workspaces)
  const switchWorkspace = useAuthStore((s) => s.switchWorkspace)

  const ownedCount = workspaces.filter((w) => w.role === 'owner').length
  const canCreate = ownedCount < 5

  const [displayName, setDisplayName] = useState('')
  const [wsType, setWsType] = useState<WorkspaceType>('personal')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (!displayName.trim() || loading || !canCreate) return
    setLoading(true)
    setError(null)
    try {
      const ws = await workspacesAPI.create({ display_name: displayName.trim(), workspace_type: wsType })
      const { access_token, workspace_id } = await workspacesAPI.switch(ws.id)
      switchWorkspace(workspace_id, access_token)
      router.push('/dashboard')
      router.refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create workspace')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="max-w-xl mx-auto p-4 sm:p-8">
      <div className="mb-8">
        <h1 className="text-xl font-semibold text-gray-900">New workspace</h1>
        <p className="text-sm text-gray-500 mt-1">
          You can create up to 5 workspaces. You can be a member of unlimited workspaces others create.
        </p>
        {!canCreate && (
          <div className="mt-3 text-sm text-amber-700 bg-amber-50 border border-amber-200 rounded-lg px-3 py-2">
            You already own 5 workspaces — the maximum allowed.
          </div>
        )}
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        <div className="space-y-1.5">
          <label htmlFor="display_name" className="text-sm font-medium text-gray-700">Workspace name</label>
          <input
            id="display_name"
            type="text"
            placeholder="Acme Corp, My Project, …"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            disabled={!canCreate || loading}
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-purple-500 focus:border-transparent disabled:opacity-50"
          />
        </div>

        <div className="space-y-1.5">
          <p className="text-sm font-medium text-gray-700">Type</p>
          <div className="grid grid-cols-1 gap-2">
            {TYPES.map(({ value, label, description, icon: Icon }) => (
              <button
                key={value}
                type="button"
                onClick={() => setWsType(value)}
                disabled={!canCreate || loading}
                className={cn(
                  'flex items-start gap-3 rounded-xl border p-3.5 text-left transition-all',
                  wsType === value
                    ? 'border-purple-500 bg-purple-50 ring-1 ring-purple-500/40'
                    : 'border-gray-200 hover:border-gray-300 hover:bg-gray-50'
                )}
              >
                <div className={cn(
                  'w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0',
                  wsType === value ? 'bg-purple-100' : 'bg-gray-100'
                )}>
                  <Icon className={cn('w-4 h-4', wsType === value ? 'text-purple-600' : 'text-gray-500')} />
                </div>
                <div>
                  <p className={cn('text-sm font-medium', wsType === value ? 'text-purple-900' : 'text-gray-800')}>
                    {label}
                  </p>
                  <p className="text-xs text-gray-500 mt-0.5">{description}</p>
                </div>
              </button>
            ))}
          </div>
        </div>

        {error && (
          <p className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">{error}</p>
        )}

        <div className="flex gap-3">
          <button
            type="submit"
            disabled={!displayName.trim() || !canCreate || loading}
            className="px-4 py-2 rounded-lg bg-purple-600 text-white text-sm font-medium hover:bg-purple-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {loading ? 'Creating…' : 'Create workspace'}
          </button>
          <button
            type="button"
            onClick={() => router.back()}
            disabled={loading}
            className="px-4 py-2 rounded-lg border border-gray-300 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50 transition-colors"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  )
}
