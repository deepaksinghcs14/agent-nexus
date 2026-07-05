'use client'

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Users } from 'lucide-react'
import { adminAPI } from '@/lib/api'
import { relativeTime, statusColor } from '@/lib/utils'
import type { User } from '@/types'

export default function AdminUsersPage() {
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const { data, isLoading, error } = useQuery({ queryKey: ['admin-users'], queryFn: () => adminAPI.users() as Promise<{ data: User[] }> })
  const update = useMutation({ mutationFn: (user: User) => adminAPI.updateUser(user.id, { is_active: !user.is_active }), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-users'] }) })
  const users = (data?.data ?? []).filter((user) => `${user.full_name} ${user.email}`.toLowerCase().includes(search.toLowerCase()))
  return (
    <div className="p-4 sm:p-6">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-4">
        <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">Users ({users.length})</h1>
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search users…"
          className="text-[12px] px-3 py-1.5 border border-gray-200 dark:border-gray-700 rounded-lg w-48"
        />
      </div>
      {error && <ErrorMessage error={error as Error} />}
      {isLoading && <Loading />}
      {!isLoading && !error && users.length === 0 && <Empty icon={Users} text="No users found." />}
      <div className="overflow-x-auto -mx-4 sm:mx-0">
        <div className="min-w-[600px] px-4 sm:px-0">
          <div className="bg-white dark:bg-gray-900 border border-gray-100 dark:border-gray-800 rounded-xl overflow-hidden">
            {users.map((user) => (
              <div key={user.id} className="grid grid-cols-12 items-center px-4 py-3 border-b last:border-b-0 border-gray-50 text-[12px]">
                <div className="col-span-5">
                  <p className="font-medium text-gray-900 dark:text-gray-100">{user.full_name || user.email}</p>
                  <p className="text-[10px] text-gray-400 dark:text-gray-500">{user.email}</p>
                </div>
                <span className="col-span-2 text-gray-500 dark:text-gray-400">{user.is_admin ? 'admin' : 'member'}</span>
                <span className="col-span-2">
                  <span className={`text-[10px] px-2 py-0.5 rounded-full ${statusColor(user.is_active ? 'active' : 'failed')}`}>
                    {user.is_active ? 'active' : 'disabled'}
                  </span>
                </span>
                <span className="col-span-2 text-[10px] text-gray-400 dark:text-gray-500">{relativeTime(user.updated_at)}</span>
                <button onClick={() => update.mutate(user)} className="col-span-1 text-[10px] text-purple-600 dark:text-purple-300">
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

function ErrorMessage({ error }: { error: Error }) { return <div className="text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg p-3 mb-4">{error.message}</div> }
function Loading() { return <div className="py-12 text-center text-sm text-gray-400 dark:text-gray-500">Loading…</div> }
function Empty({ icon: Icon, text }: { icon: React.ElementType; text: string }) { return <div className="border border-dashed border-gray-200 dark:border-gray-700 rounded-xl py-12 text-center"><Icon className="mx-auto text-gray-300 dark:text-gray-600 mb-3" /><p className="text-sm text-gray-500 dark:text-gray-400">{text}</p></div> }
