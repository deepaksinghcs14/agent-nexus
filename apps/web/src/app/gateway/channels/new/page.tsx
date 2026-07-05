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
  const [accountId, setAccountId] = useState('')
  const [accountIdTouched, setAccountIdTouched] = useState(false)
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

  // Auto-derive account ID from channel name unless the user has manually set it.
  const derivedAccountId = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'channel'
  const effectiveAccountId = accountIdTouched ? accountId : derivedAccountId

  const save = async () => {
    if (!name.trim() || !agentId) {
      setError('Name and agent are required')
      return
    }
    const config = {
      account_id: effectiveAccountId || derivedAccountId,
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
    <div className="p-4 sm:p-6 max-w-3xl">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-5">
        <div className="flex items-center gap-2 text-[12px] text-gray-400 dark:text-gray-500">
          <span onClick={() => router.push('/gateway/channels')} className="hover:text-gray-600 dark:hover:text-gray-300 cursor-pointer">Gateway</span>
          <ChevronRight className="w-3 h-3" />
          <span className="text-gray-700 dark:text-gray-300 font-medium">New channel</span>
        </div>
        <div className="flex gap-2">
          <button onClick={() => router.push('/gateway/channels')} className="inline-flex items-center gap-1.5 px-3 py-1.5 border border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400 text-[12px] rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800">
            <X className="w-3.5 h-3.5" /> Discard
          </button>
          <button onClick={save} className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-[12px] rounded-lg hover:bg-purple-700">
            <Save className="w-3.5 h-3.5" /> Create Channel
          </button>
        </div>
      </div>
      {error && <div className="mb-4 text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg px-3 py-2">{error}</div>}
      <div className="space-y-5">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {(['whatsapp', 'http'] as const).map((t) => (
            <button key={t} onClick={() => setChannelType(t)} className={`text-left border rounded-lg p-4 ${channelType === t ? 'border-purple-400 bg-purple-50 dark:bg-purple-500/10' : 'border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900'}`}>
              <div className="text-sm font-medium text-gray-900 dark:text-gray-100">{t === 'whatsapp' ? 'WhatsApp' : 'HTTP'}</div>
              <div className="text-xs text-gray-500 dark:text-gray-400 mt-1">{t === 'whatsapp' ? 'Gateway-owned WhatsApp Web adapter.' : 'Generic session-aware HTTP ingress.'}</div>
            </button>
          ))}
        </div>
        <Field label="Name"><input value={name} onChange={(e) => setName(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg" /></Field>
        <Field label="Description"><input value={description} onChange={(e) => setDescription(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg" /></Field>
        <Field label="Agent">
          <select value={agentId} onChange={(e) => setAgentId(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900">
            {agents.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
          </select>
        </Field>
        {channelType === 'whatsapp' && (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <Field label="Account ID" hint="Unique per WhatsApp number — different channels sharing the same number use the same ID">
                <input
                  value={accountIdTouched ? accountId : derivedAccountId}
                  onChange={(e) => { setAccountIdTouched(true); setAccountId(e.target.value) }}
                  placeholder={derivedAccountId}
                  className="w-full text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg"
                />
              </Field>
              <Field label="Internal adapter URL"><input value={adapterURL} onChange={(e) => setAdapterURL(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg" /></Field>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <Field label="DM policy"><select value={dmPolicy} onChange={(e) => setDMPolicy(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900"><option value="pairing">Pairing</option><option value="allowlist">Allowlist</option><option value="open">Open</option><option value="disabled">Disabled</option></select></Field>
              <Field label="Group policy"><select value={groupPolicy} onChange={(e) => setGroupPolicy(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900"><option value="disabled">Disabled</option><option value="allowlist">Allowlist</option><option value="open">Open</option></select></Field>
            </div>
          </>
        )}
        <div>
          <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300 mb-2">Suggested skills</p>
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 divide-y divide-gray-100 dark:divide-gray-800">
            {skills.filter((s) => ['WhatsApp Formatter', 'Language Mirror', 'Safety Guardrail'].includes(s.name)).map((s) => (
              <label key={s.id} className="flex items-start gap-3 p-3 text-sm">
                <input type="checkbox" checked={!!selectedSkills[s.id]} onChange={(e) => setSelectedSkills((m) => ({ ...m, [s.id]: e.target.checked }))} className="mt-1" />
                <span><span className="font-medium text-gray-800 dark:text-gray-200">{s.name}</span><span className="block text-xs text-gray-500 dark:text-gray-400">{s.description}</span></span>
              </label>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="block text-[12px] font-medium text-gray-700 dark:text-gray-300 mb-1.5">{label}</span>
      {children}
      {hint && <span className="block text-[11px] text-gray-400 dark:text-gray-500 mt-1">{hint}</span>}
    </label>
  )
}
