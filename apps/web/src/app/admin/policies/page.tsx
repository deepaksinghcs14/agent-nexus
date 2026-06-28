'use client'

import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save, Shield } from 'lucide-react'
import { adminAPI } from '@/lib/api'
import type { Policy } from '@/types'

export default function AdminPoliciesPage() {
  const queryClient = useQueryClient()
  const [values, setValues] = useState<Record<string, string>>({})
  const [actionError, setActionError] = useState('')
  const { data, isLoading, error } = useQuery({ queryKey: ['admin-policies'], queryFn: () => adminAPI.policies() as Promise<{ data: Policy[] }> })
  const policies = data?.data ?? []
  useEffect(() => { setValues(Object.fromEntries(policies.map((policy) => [policy.key, JSON.stringify(policy.value, null, 2)]))) }, [data])
  const save = useMutation({
    mutationFn: () => adminAPI.setPolicies({ policies: Object.entries(values).map(([key, value]) => ({ key, value: JSON.parse(value) })) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['admin-policies'] }),
    onError: (err: Error) => setActionError(err.message),
  })
  return (
    <div className="p-4 sm:p-6 max-w-2xl">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-5">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Platform policies</h1>
          <p className="text-sm text-gray-500 mt-0.5">Global policy values used by the runtime</p>
        </div>
        <button
          onClick={() => {
            try {
              Object.values(values).forEach((value) => JSON.parse(value))
              save.mutate()
            } catch {
              setActionError('Every policy value must be valid JSON')
            }
          }}
          disabled={!policies.length || save.isPending}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-[12px] rounded-lg disabled:opacity-50"
        >
          <Save className="w-3.5 h-3.5" /> {save.isPending ? 'Saving…' : 'Save changes'}
        </button>
      </div>
      {(error || actionError) && (
        <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-3 mb-4">
          {actionError || (error as Error).message}
        </div>
      )}
      {isLoading && <div className="py-12 text-center text-sm text-gray-400">Loading policies…</div>}
      {!isLoading && !error && policies.length === 0 && (
        <div className="border border-dashed border-gray-200 rounded-xl py-12 text-center">
          <Shield className="mx-auto text-gray-300 mb-3" />
          <p className="text-sm text-gray-500">No policies configured.</p>
        </div>
      )}
      <div className="space-y-3">
        {policies.map((policy) => (
          <div key={policy.id || policy.key} className="bg-white border border-gray-100 rounded-xl p-4">
            <label className="text-[12px] font-medium text-gray-800 block mb-2">{policy.key}</label>
            <textarea
              value={values[policy.key] ?? ''}
              onChange={(e) => setValues({ ...values, [policy.key]: e.target.value })}
              rows={3}
              className="w-full text-xs font-mono px-3 py-2 border border-gray-200 rounded-lg"
            />
          </div>
        ))}
      </div>
    </div>
  )
}
