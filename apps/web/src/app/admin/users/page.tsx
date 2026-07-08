'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Users } from 'lucide-react'
import { adminAPI } from '@/lib/api'
import { relativeTime, statusColor } from '@/lib/utils'
import { PageHeader } from '@/components/ui/PageHeader'
import type { User } from '@/types'

export default function AdminUsersPage() {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const { data, isLoading, error } = useQuery({ queryKey: ['admin-users'], queryFn: () => adminAPI.users() as Promise<{ data: User[] }> })
  const update = useMutation({ mutationFn: (user: User) => adminAPI.updateUser(user.id, { is_active: !user.is_active }), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-users'] }) })
  const users = (data?.data ?? []).filter((user) => `${user.full_name} ${user.email}`.toLowerCase().includes(search.toLowerCase()))
  return (
    <div className="p-4 sm:p-6 max-w-6xl">
      <PageHeader
        eyebrow="Admin"
        title={`Users (${users.length})`}
        actions={
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search users…"
            className="text-[12px] px-3 py-2 border border-border-strong rounded-[10px] w-48 bg-surface"
          />
        }
      />
      {error && <ErrorMessage error={error as Error} />}
      {isLoading && <Loading />}
      {!isLoading && !error && users.length === 0 && <Empty icon={Users} text="No users found." />}
      <div className="overflow-x-auto -mx-4 sm:mx-0">
        <div className="min-w-[600px] px-4 sm:px-0">
          <div className="bg-surface border border-border rounded-xl overflow-hidden">
            {users.map((user) => (
              <div key={user.id} className="grid grid-cols-12 items-center px-4 py-3 border-b last:border-b-0 border-border text-[12px]">
                <div className="col-span-5">
                  <p className="font-medium text-foreground">{user.full_name || user.email}</p>
                  <p className="text-[10px] text-faint">{user.email}</p>
                </div>
                <span className="col-span-2 text-muted-foreground">{user.is_admin ? 'admin' : 'member'}</span>
                <span className="col-span-2">
                  <span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(user.is_active ? 'active' : 'failed')}`}>
                    {user.is_active ? 'active' : 'disabled'}
                  </span>
                </span>
                <span className="col-span-2 text-[10px] text-faint">{relativeTime(user.updated_at)}</span>
                <button onClick={() => update.mutate(user)} className="col-span-1 text-[10px] text-accent dark:text-accent-bright">
                  {user.is_active ? 'Disable' : 'Enable'}
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function ErrorMessage({ error }: { error: Error }) { return <div className="text-sm text-crit bg-crit/10 border border-crit/30 rounded-lg p-3 mb-4">{error.message}</div> }
function Loading() { return <div className="py-12 text-center text-sm text-faint">Loading…</div> }
function Empty({ icon: Icon, text }: { icon: React.ElementType; text: string }) { return <div className="border border-dashed border-border-strong rounded-xl py-12 text-center"><Icon className="mx-auto text-faint mb-3" /><p className="text-sm text-muted-foreground">{text}</p></div> }
