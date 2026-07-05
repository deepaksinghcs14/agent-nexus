'use client'

import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Plus, Trash2, User as UserIcon } from 'lucide-react'
import { workspaceAPI } from '@/lib/api'
import { useAuthStore } from '@/store/auth'
import type { Workspace, WorkspaceMember, WorkspaceRole, WorkspaceType } from '@/types'

const WORKSPACE_TYPES: { value: WorkspaceType; label: string }[] = [
  { value: 'personal', label: 'Personal' },
  { value: 'team', label: 'Team' },
  { value: 'organization', label: 'Organization' },
  { value: 'project', label: 'Project' },
  { value: 'sandbox', label: 'Sandbox' },
]

const editableRoles: WorkspaceRole[] = ['admin', 'member', 'viewer']

export default function WorkspaceSettingsPage() {
  const queryClient = useQueryClient()
  const { workspace, setWorkspace, user } = useAuthStore()
  const [displayName, setDisplayName] = useState('')
  const [wsType, setWsType] = useState<WorkspaceType>('personal')
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<WorkspaceRole>('member')
  const [message, setMessage] = useState('')

  const workspaceQuery = useQuery({
    queryKey: ['workspace'],
    queryFn: () => workspaceAPI.get() as Promise<{ workspace: Workspace; role: WorkspaceRole }>,
  })

  const membersQuery = useQuery({
    queryKey: ['workspace-members'],
    queryFn: () => workspaceAPI.members() as Promise<{ data: WorkspaceMember[] }>,
  })

  const activeWorkspace = workspaceQuery.data?.workspace ?? workspace
  const currentRole = workspaceQuery.data?.role
  const canManage = currentRole === 'owner' || currentRole === 'admin' || user?.is_admin
  const members = membersQuery.data?.data ?? []

  useEffect(() => {
    if (activeWorkspace) {
      setDisplayName(activeWorkspace.display_name)
      setWsType((activeWorkspace.workspace_type ?? 'personal') as WorkspaceType)
    }
  }, [activeWorkspace])

  const updateWorkspace = useMutation({
    mutationFn: () => workspaceAPI.update({ display_name: displayName.trim(), workspace_type: wsType }),
    onSuccess: (data) => {
      setWorkspace(data as Workspace)
      queryClient.invalidateQueries({ queryKey: ['workspace'] })
      setMessage('Workspace saved')
    },
    onError: (err: Error) => setMessage(err.message),
  })

  const addMember = useMutation({
    mutationFn: () => workspaceAPI.addMember({ email, role }),
    onSuccess: () => {
      setEmail('')
      setRole('member')
      setMessage('Member added')
      queryClient.invalidateQueries({ queryKey: ['workspace-members'] })
    },
    onError: (err: Error) => setMessage(err.message),
  })

  const updateMember = useMutation({
    mutationFn: ({ id, nextRole }: { id: string; nextRole: WorkspaceRole }) =>
      workspaceAPI.updateMember(id, { role: nextRole }),
    onSuccess: () => {
      setMessage('Role updated')
      queryClient.invalidateQueries({ queryKey: ['workspace-members'] })
    },
    onError: (err: Error) => setMessage(err.message),
  })

  const removeMember = useMutation({
    mutationFn: (id: string) => workspaceAPI.removeMember(id),
    onSuccess: () => {
      setMessage('Member removed')
      queryClient.invalidateQueries({ queryKey: ['workspace-members'] })
    },
    onError: (err: Error) => setMessage(err.message),
  })

  return (
    <div className="p-4 sm:p-6 max-w-4xl">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">Workspace</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">Manage your workspace settings and members</p>
      </div>

      {message && (
        <div className={`text-sm border rounded-lg p-3 mb-4 ${
          message.includes('saved') || message.includes('added') || message.includes('updated') || message.includes('removed')
            ? 'text-green-700 dark:text-green-300 bg-green-50 dark:bg-green-500/10 border-green-200'
            : 'text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border-red-200'
        }`}>
          {message}
        </div>
      )}

      <div className="space-y-5">
        <section className="bg-white dark:bg-gray-900 border border-gray-100 dark:border-gray-800 rounded-xl p-6">
          <h2 className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-4">General</h2>
          <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1">Workspace name</label>
          <input
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            disabled={!canManage}
            className="w-full px-3 py-2 text-sm border border-gray-200 dark:border-gray-700 rounded-lg disabled:bg-gray-50"
          />
          <label className="block text-xs font-medium text-gray-700 dark:text-gray-300 mt-4 mb-1.5">Workspace type</label>
          <div className="flex flex-wrap gap-2">
            {WORKSPACE_TYPES.map(({ value, label }) => (
              <button
                key={value}
                type="button"
                disabled={!canManage}
                onClick={() => setWsType(value)}
                className={`px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors disabled:opacity-50 ${
                  wsType === value
                    ? 'border-purple-500 bg-purple-50 dark:bg-purple-500/10 text-purple-700 dark:text-purple-300'
                    : 'border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400 hover:border-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800'
                }`}
              >
                {label}
              </button>
            ))}
          </div>
          <div className="flex flex-wrap items-center gap-3 justify-between mt-3">
            <p className="text-[11px] text-gray-400 dark:text-gray-500">{activeWorkspace?.name}</p>
            <button
              onClick={() => updateWorkspace.mutate()}
              disabled={!canManage || !displayName.trim() || updateWorkspace.isPending}
              className="inline-flex items-center gap-1.5 px-4 py-2 bg-purple-600 hover:bg-purple-700 text-white text-sm rounded-lg disabled:opacity-50"
            >
              <Check className="w-3.5 h-3.5" /> Save
            </button>
          </div>
        </section>

        <section className="bg-white dark:bg-gray-900 border border-gray-100 dark:border-gray-800 rounded-xl p-6">
          <div className="flex items-center justify-between gap-4 mb-4">
            <h2 className="text-sm font-medium text-gray-900 dark:text-gray-100">Members</h2>
            <span className="text-xs text-gray-400 dark:text-gray-500">{members.length} members</span>
          </div>

          {canManage && (
            <div className="grid grid-cols-1 sm:grid-cols-[1fr_140px_auto] gap-2 mb-5">
              <input
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="registered-user@example.com"
                className="px-3 py-2 text-sm border border-gray-200 dark:border-gray-700 rounded-lg"
              />
              <select
                value={role}
                onChange={(e) => setRole(e.target.value as WorkspaceRole)}
                className="px-3 py-2 text-sm border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900"
              >
                {editableRoles.map((item) => <option key={item} value={item}>{item}</option>)}
              </select>
              <button
                onClick={() => addMember.mutate()}
                disabled={!email.trim() || addMember.isPending}
                className="inline-flex items-center gap-1.5 px-4 py-2 bg-gray-900 text-white text-sm rounded-lg disabled:opacity-50"
              >
                <Plus className="w-3.5 h-3.5" /> Add
              </button>
            </div>
          )}

          <div className="divide-y divide-gray-100 dark:divide-gray-800 border border-gray-100 dark:border-gray-800 rounded-lg overflow-hidden">
            {membersQuery.isLoading && <div className="p-4 text-sm text-gray-400 dark:text-gray-500">Loading members...</div>}
            {!membersQuery.isLoading && members.length === 0 && <div className="p-4 text-sm text-gray-400 dark:text-gray-500">No members found.</div>}
            {members.map((member) => (
              <div key={member.id} className="grid grid-cols-[1fr_auto] sm:grid-cols-[1fr_150px_40px] gap-3 items-center px-4 py-3">
                <div className="flex items-center gap-3 min-w-0">
                  <div className="w-8 h-8 rounded-full bg-gray-100 dark:bg-gray-800 flex items-center justify-center flex-shrink-0">
                    <UserIcon className="w-4 h-4 text-gray-500 dark:text-gray-400" />
                  </div>
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-gray-800 dark:text-gray-200 truncate">{member.full_name || member.email}</p>
                    <p className="text-xs text-gray-400 dark:text-gray-500 truncate">{member.email}</p>
                  </div>
                </div>
                <select
                  value={member.role}
                  disabled={!canManage || member.role === 'owner'}
                  onChange={(e) => updateMember.mutate({ id: member.id, nextRole: e.target.value as WorkspaceRole })}
                  className="px-2 py-1.5 text-sm border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900 disabled:bg-gray-50"
                >
                  {member.role === 'owner' && <option value="owner">owner</option>}
                  {editableRoles.map((item) => <option key={item} value={item}>{item}</option>)}
                </select>
                <button
                  onClick={() => removeMember.mutate(member.id)}
                  disabled={!canManage || member.role === 'owner'}
                  className="w-8 h-8 inline-flex items-center justify-center rounded-lg text-gray-400 dark:text-gray-500 hover:text-red-600 hover:bg-red-50 disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-gray-400 sm:justify-self-auto"
                  aria-label={`Remove ${member.email}`}
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            ))}
          </div>
        </section>
      </div>
    </div>
  )
}
