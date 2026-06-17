'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { ChevronRight, Save, X } from 'lucide-react'
import { agentsAPI, gatewayAPI, skillsAPI } from '@/lib/api'
import type { Agent, Skill } from '@/types'

export default function NewGatewayChannelPage() {
  const router = useRouter()
  const [agents, setAgents] = useState<Agent[]>([])
  const [skills, setSkills] = useState<Skill[]>([])
  const [channelType, setChannelType] = useState<'whatsapp' | 'http'>('whatsapp')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [agentId, setAgentId] = useState('')
  const [accountId, setAccountId] = useState('default')
  const [adapterURL, setAdapterURL] = useState('http://127.0.0.1:18901')
  const [dmPolicy, setDMPolicy] = useState('pairing')
  const [groupPolicy, setGroupPolicy] = useState('disabled')
  const [selectedSkills, setSelectedSkills] = useState<Record<string, boolean>>({})
  const [error, setError] = useState('')

  useEffect(() => {
    agentsAPI.list().then((r) => {
      const data = ((r as { data?: Agent[] }).data) ?? []
      setAgents(data)
      setAgentId((current) => current || data[0]?.id || '')
    }).catch(() => {})
    skillsAPI.list().then((r) => {
      const data = ((r as { data?: Skill[] }).data) ?? []
      setSkills(data)
      const defaults = ['WhatsApp Formatter', 'Language Mirror', 'Safety Guardrail']
      setSelectedSkills(Object.fromEntries(data.filter((s) => defaults.includes(s.name)).map((s) => [s.id, true])))
    }).catch(() => {})
  }, [])

  const save = async () => {
    if (!name.trim() || !agentId) {
      setError('Name and agent are required')
      return
    }
    const config = {
      account_id: accountId || 'default',
      adapter_url: channelType === 'whatsapp' ? adapterURL : undefined,
      dm_policy: dmPolicy,
      session_scope: 'per-channel-peer',
      group_policy: groupPolicy,
      history_limit: 50,
      self_chat_enabled: false,
      allow_from: [],
      group_allow_from: [],
    }
    const channel = await gatewayAPI.createChannel({ name, description, agent_id: agentId, channel_type: channelType, config }) as { id: string }
    const assignments = Object.entries(selectedSkills)
      .filter(([, enabled]) => enabled)
      .map(([skill_id], i) => ({ skill_id, enabled: true, order_index: i }))
    if (assignments.length > 0) {
      await skillsAPI.setForAgent(agentId, { skills: assignments }).catch(() => {})
    }
    router.push(`/gateway/channels/${channel.id}`)
  }

  return (
    <div className="p-6 max-w-3xl">
      <div className="flex items-center justify-between mb-5">
        <div className="flex items-center gap-2 text-[12px] text-gray-400">
          <span onClick={() => router.push('/gateway/channels')} className="hover:text-gray-600 cursor-pointer">Gateway</span>
          <ChevronRight className="w-3 h-3" />
          <span className="text-gray-700 font-medium">New channel</span>
        </div>
        <div className="flex gap-2">
          <button onClick={() => router.push('/gateway/channels')} className="inline-flex items-center gap-1.5 px-3 py-1.5 border border-gray-200 text-gray-600 text-[12px] rounded-lg hover:bg-gray-50">
            <X className="w-3.5 h-3.5" /> Discard
          </button>
          <button onClick={save} className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-[12px] rounded-lg hover:bg-purple-700">
            <Save className="w-3.5 h-3.5" /> Create Channel
          </button>
        </div>
      </div>
      {error && <div className="mb-4 text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">{error}</div>}
      <div className="space-y-5">
        <div className="grid grid-cols-2 gap-3">
          {(['whatsapp', 'http'] as const).map((t) => (
            <button key={t} onClick={() => setChannelType(t)} className={`text-left border rounded-lg p-4 ${channelType === t ? 'border-purple-400 bg-purple-50' : 'border-gray-200 bg-white'}`}>
              <div className="text-sm font-medium text-gray-900">{t === 'whatsapp' ? 'WhatsApp' : 'HTTP'}</div>
              <div className="text-xs text-gray-500 mt-1">{t === 'whatsapp' ? 'Gateway-owned WhatsApp Web adapter.' : 'Generic session-aware HTTP ingress.'}</div>
            </button>
          ))}
        </div>
        <Field label="Name"><input value={name} onChange={(e) => setName(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg" /></Field>
        <Field label="Description"><input value={description} onChange={(e) => setDescription(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg" /></Field>
        <Field label="Agent">
          <select value={agentId} onChange={(e) => setAgentId(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white">
            {agents.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
          </select>
        </Field>
        {channelType === 'whatsapp' && (
          <>
            <div className="grid grid-cols-2 gap-4">
              <Field label="Account ID"><input value={accountId} onChange={(e) => setAccountId(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg" /></Field>
              <Field label="Internal adapter URL"><input value={adapterURL} onChange={(e) => setAdapterURL(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg" /></Field>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <Field label="DM policy"><select value={dmPolicy} onChange={(e) => setDMPolicy(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white"><option value="pairing">Pairing</option><option value="allowlist">Allowlist</option><option value="open">Open</option><option value="disabled">Disabled</option></select></Field>
              <Field label="Group policy"><select value={groupPolicy} onChange={(e) => setGroupPolicy(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white"><option value="disabled">Disabled</option><option value="allowlist">Allowlist</option><option value="open">Open</option></select></Field>
            </div>
          </>
        )}
        <div>
          <p className="text-[12px] font-medium text-gray-700 mb-2">Suggested skills</p>
          <div className="rounded-lg border border-gray-200 divide-y divide-gray-100">
            {skills.filter((s) => ['WhatsApp Formatter', 'Language Mirror', 'Safety Guardrail'].includes(s.name)).map((s) => (
              <label key={s.id} className="flex items-start gap-3 p-3 text-sm">
                <input type="checkbox" checked={!!selectedSkills[s.id]} onChange={(e) => setSelectedSkills((m) => ({ ...m, [s.id]: e.target.checked }))} className="mt-1" />
                <span><span className="font-medium text-gray-800">{s.name}</span><span className="block text-xs text-gray-500">{s.description}</span></span>
              </label>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="block"><span className="block text-[12px] font-medium text-gray-700 mb-1.5">{label}</span>{children}</label>
}
