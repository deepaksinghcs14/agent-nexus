'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { useMutation, useQuery } from '@tanstack/react-query'
import { agentsAPI, groupsAPI } from '@/lib/api'
import type { Agent } from '@/types'

export default function NewAgentGroupPage() {
  const router = useRouter()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [mode, setMode] = useState<'pipeline' | 'supervisor'>('pipeline')
  const [agentIds, setAgentIds] = useState<string[]>([])
  const [error, setError] = useState('')
  const { data } = useQuery({ queryKey: ['agents'], queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }> })
  const agents = data?.data ?? []
  const create = useMutation({
    mutationFn: () => groupsAPI.create({ name, description, mode, agent_ids: agentIds }),
    onSuccess: () => router.push('/agent-groups'),
    onError: (err: Error) => setError(err.message),
  })

  return <div className="p-6 max-w-2xl">
    <div className="mb-6"><h1 className="text-lg font-semibold text-gray-900">Create Agent Group</h1><p className="text-sm text-gray-500 mt-0.5">Orchestrate multiple agents in a pipeline or supervisor workflow</p></div>
    <div className="bg-white border border-gray-100 rounded-xl p-6 space-y-4">
      {error && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-2">{error}</div>}
      <Field label="Group name"><input value={name} onChange={(e) => setName(e.target.value)} className="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg" placeholder="e.g. Research pipeline" /></Field>
      <Field label="Description"><textarea value={description} onChange={(e) => setDescription(e.target.value)} className="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg resize-none" rows={2} placeholder="What does this group do?" /></Field>
      <Field label="Mode"><select value={mode} onChange={(e) => setMode(e.target.value as typeof mode)} className="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg bg-white"><option value="pipeline">Pipeline</option><option value="supervisor">Supervisor</option></select></Field>
      <Field label="Agents"><div className="space-y-2">{agents.map((agent) => <label key={agent.id} className="flex items-center gap-2 text-sm text-gray-700"><input type="checkbox" checked={agentIds.includes(agent.id)} onChange={() => setAgentIds((ids) => ids.includes(agent.id) ? ids.filter((id) => id !== agent.id) : [...ids, agent.id])} />{agent.name}</label>)}{agents.length === 0 && <p className="text-sm text-gray-400">Create agents before building a group.</p>}</div></Field>
      <div className="pt-2 flex gap-2 justify-end"><Link href="/agent-groups" className="px-4 py-2 text-sm text-gray-600 border border-gray-200 rounded-lg">Cancel</Link><button onClick={() => create.mutate()} disabled={!name.trim() || !agentIds.length || create.isPending} className="px-4 py-2 bg-[#534AB7] text-white text-sm font-medium rounded-lg disabled:opacity-50">{create.isPending ? 'Creating…' : 'Create group'}</button></div>
    </div>
  </div>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <div><label className="block text-xs font-medium text-gray-700 mb-1">{label}</label>{children}</div>
}
