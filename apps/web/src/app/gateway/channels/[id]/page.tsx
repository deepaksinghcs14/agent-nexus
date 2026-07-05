'use client'

import { use, useCallback, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import Image from 'next/image'
import { Check, ChevronRight, Copy, LogOut, Pencil, QrCode, RefreshCw, X } from 'lucide-react'
import { agentsAPI, gatewayAPI } from '@/lib/api'
import type { ChannelSession, GatewayChannel, GatewayContact, GatewayEscalation, GatewayEvent, GatewayOutboundMessage, GatewayPairingRequest, GatewayReminder, ScheduledMessage } from '@/types'

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'
const WA_TABS = ['Overview', 'QR Login', 'Contacts', 'Sessions', 'Pairing', 'Reminders', 'Scheduled', 'Escalations', 'Events', 'Outbox'] as const
const HTTP_TABS = ['Overview', 'Sessions', 'Events', 'Outbox', 'Reminders', 'Scheduled'] as const
type Tab = typeof WA_TABS[number]

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button onClick={() => { navigator.clipboard.writeText(text); setCopied(true); setTimeout(() => setCopied(false), 1600) }} className="p-1 rounded hover:bg-gray-100 dark:hover:bg-gray-800 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300" title="Copy">
      {copied ? <Check className="w-3.5 h-3.5 text-green-600 dark:text-green-300" /> : <Copy className="w-3.5 h-3.5" />}
    </button>
  )
}

export default function GatewayChannelDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id: channelId } = use(params)
  const [tab, setTab] = useState<Tab>('Overview')
  const [channel, setChannel] = useState<GatewayChannel | null>(null)
  const [sessions, setSessions] = useState<ChannelSession[]>([])
  const [events, setEvents] = useState<GatewayEvent[]>([])
  const [pairings, setPairings] = useState<GatewayPairingRequest[]>([])
  const [outbox, setOutbox] = useState<GatewayOutboundMessage[]>([])
  const [contacts, setContacts] = useState<GatewayContact[]>([])
  const [contactTotal, setContactTotal] = useState(0)
  const [contactPage, setContactPage] = useState(1)
  const [contactSearch, setContactSearch] = useState('')
  const [contactSearchInput, setContactSearchInput] = useState('')
  const [reminders, setReminders] = useState<GatewayReminder[]>([])
  const [scheduledMessages, setScheduledMessages] = useState<ScheduledMessage[]>([])
  const [escalations, setEscalations] = useState<GatewayEscalation[]>([])
  const [adapter, setAdapter] = useState<Record<string, unknown> | null>(null)
  const [qr, setQR] = useState<Record<string, unknown> | null>(null)
  const [loginLoading, setLoginLoading] = useState(false)
  const [error, setError] = useState('')
  const [contactName, setContactName] = useState('')
  const [contactPhone, setContactPhone] = useState('')
  const [contactRole, setContactRole] = useState<'owner' | 'trusted' | 'blocked'>('trusted')
  const [contactAgentId, setContactAgentId] = useState('')
  const [agents, setAgents] = useState<{ id: string; name: string }[]>([])
  const [schedContact, setSchedContact] = useState('')
  const [schedMsg, setSchedMsg] = useState('')
  const [schedSendAt, setSchedSendAt] = useState('')
  const [schedFreq, setSchedFreq] = useState('')
  const [schedInterval, setSchedInterval] = useState(1)
  const [schedEndAt, setSchedEndAt] = useState('')
  const [schedMaxOcc, setSchedMaxOcc] = useState('')
  const [schedUseAgent, setSchedUseAgent] = useState(false)
  const [testInput, setTestInput] = useState('')
  const [testSessionId, setTestSessionId] = useState('')
  const [testResponse, setTestResponse] = useState<string | null>(null)
  const [testLoading, setTestLoading] = useState(false)

  const [syncLIDsLoading, setSyncLIDsLoading] = useState(false)
  const [syncLIDsResult, setSyncLIDsResult] = useState<string | null>(null)
  const [syncContactsLoading, setSyncContactsLoading] = useState(false)
  const [syncContactsResult, setSyncContactsResult] = useState<string | null>(null)

  const [pairingPhone, setPairingPhone] = useState('')
  const [pairingCode, setPairingCode] = useState('')
  const [pairingLoading, setPairingLoading] = useState(false)

  const [editMode, setEditMode] = useState(false)
  const [editName, setEditName] = useState('')
  const [editDesc, setEditDesc] = useState('')
  const [editAgentId, setEditAgentId] = useState('')
  const [editIsActive, setEditIsActive] = useState(true)
  const [editDmPolicy, setEditDmPolicy] = useState('')
  const [editGroupPolicy, setEditGroupPolicy] = useState('')
  const [editHistoryLimit, setEditHistoryLimit] = useState('')
  const [editChatApprovals, setEditChatApprovals] = useState(false)
  const [editAdapterUrl, setEditAdapterUrl] = useState('')
  const [editBrowserName, setEditBrowserName] = useState('')
  const [editSaving, setEditSaving] = useState(false)

  const openEdit = () => {
    if (!channel) return
    setEditName(channel.name)
    setEditDesc(channel.description || '')
    setEditAgentId(channel.agent_id)
    setEditIsActive(channel.is_active)
    setEditDmPolicy(channel.config?.dm_policy || '')
    setEditGroupPolicy(channel.config?.group_policy || '')
    setEditHistoryLimit(channel.config?.history_limit != null ? String(channel.config.history_limit) : '')
    setEditChatApprovals(!!channel.config?.chat_approvals_enabled)
    setEditAdapterUrl(channel.config?.adapter_url || '')
    setEditBrowserName(channel.config?.browser_name || '')
    setEditMode(true)
  }

  const saveEdit = async () => {
    if (!channel) return
    setEditSaving(true)
    setError('')
    const configPatch: Record<string, unknown> = {}
    if (editDmPolicy) configPatch.dm_policy = editDmPolicy
    if (editGroupPolicy) configPatch.group_policy = editGroupPolicy
    if (editHistoryLimit) configPatch.history_limit = parseInt(editHistoryLimit)
    configPatch.chat_approvals_enabled = editChatApprovals
    if (editAdapterUrl) configPatch.adapter_url = editAdapterUrl
    if (editBrowserName) configPatch.browser_name = editBrowserName
    const body: Record<string, unknown> = {
      name: editName,
      description: editDesc,
      agent_id: editAgentId,
      is_active: editIsActive,
      config: { ...channel.config, ...configPatch },
    }
    const updated = await gatewayAPI.updateChannel(channel.id, body).catch((e: Error) => { setError(e.message); return null })
    if (updated) { setChannel(updated as typeof channel); setEditMode(false) }
    setEditSaving(false)
  }

  const ingressURL = useMemo(() => channel ? `${API_URL}/gateway/${channel.channel_type}/${channel.id}` : '', [channel])

  const loadContacts = useCallback((page: number, q: string) => {
    gatewayAPI.listContacts({ channel_id: channelId, page, per_page: 20, ...(q ? { q } : {}) })
      .then((r) => {
        const res = r as { data?: GatewayContact[]; total?: number }
        setContacts(res.data ?? [])
        setContactTotal(res.total ?? 0)
      }).catch(() => {})
  }, [channelId])

  const load = useCallback(() => {
    const q = `channel_id=${channelId}`
    gatewayAPI.getChannel(channelId).then((c) => {
      const ch = c as GatewayChannel
      setChannel(ch)
      if (ch.channel_type === 'whatsapp') {
        gatewayAPI.listPairings(q).then((r) => setPairings(((r as { data?: GatewayPairingRequest[] }).data) ?? [])).catch(() => {})
        loadContacts(1, '')
      }
    }).catch((e: Error) => setError(e.message))
    gatewayAPI.listSessions(q).then((r) => setSessions(((r as { data?: ChannelSession[] }).data) ?? [])).catch(() => {})
    gatewayAPI.listEvents(q).then((r) => setEvents(((r as { data?: GatewayEvent[] }).data) ?? [])).catch(() => {})
    gatewayAPI.listOutbox(q).then((r) => setOutbox(((r as { data?: GatewayOutboundMessage[] }).data) ?? [])).catch(() => {})
    gatewayAPI.listReminders(q).then((r) => setReminders(((r as { data?: GatewayReminder[] }).data) ?? [])).catch(() => {})
    gatewayAPI.listScheduledMessages(q).then((r) => setScheduledMessages(((r as { data?: ScheduledMessage[] }).data) ?? [])).catch(() => {})
    gatewayAPI.listEscalations(q).then((r) => setEscalations(((r as { data?: GatewayEscalation[] }).data) ?? [])).catch(() => {})
    agentsAPI.list().then((r) => setAgents(((r as { data?: { id: string; name: string }[] }).data) ?? [])).catch(() => {})
  }, [channelId])

  useEffect(() => { load() }, [load])

  // Reload contacts when page or search term changes
  useEffect(() => { loadContacts(contactPage, contactSearch) }, [contactPage, contactSearch, loadContacts])

  // Debounce search input → contactSearch
  useEffect(() => {
    const t = setTimeout(() => { setContactSearch(contactSearchInput); setContactPage(1) }, 300)
    return () => clearTimeout(t)
  }, [contactSearchInput])

  const refreshAdapter = useCallback(async () => {
    const r = await gatewayAPI.adapterStatus(channelId).catch(() => null)
    if (r) setAdapter(r as Record<string, unknown>)
    return r as Record<string, unknown> | null
  }, [channelId])

  const startLogin = async () => {
    setError('')
    setLoginLoading(true)
    setQR(null)
    try {
      await gatewayAPI.startLogin(channelId)
      // Poll for QR or connected status
      for (let i = 0; i < 20; i++) {
        await new Promise((res) => setTimeout(res, 800))
        const status = await refreshAdapter()
        if (status?.status === 'qr' && status?.qr_data_url) {
          setQR(status)
          break
        }
        if (status?.status === 'connected') break
      }
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setLoginLoading(false)
    }
  }

  const getPairingCode = async () => {
    if (!pairingPhone.trim()) return
    setPairingLoading(true)
    setPairingCode('')
    setError('')
    try {
      await gatewayAPI.startLogin(channelId)
      await new Promise((res) => setTimeout(res, 1500))
      const r = await gatewayAPI.requestPairingCode(channelId, pairingPhone.trim())
      setPairingCode((r as { code?: string }).code ?? '')
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setPairingLoading(false)
    }
  }

  // Auto-poll while in qr or connecting state
  useEffect(() => {
    if (!adapter) return
    const s = adapter.status as string
    if (s !== 'qr' && s !== 'connecting') return
    const interval = setInterval(async () => {
      const r = await refreshAdapter()
      if (r?.status === 'connected' || r?.status === 'disconnected') {
        setQR(null)
        clearInterval(interval)
      } else if (r?.status === 'qr') {
        setQR(r)
      }
    }, 3000)
    return () => clearInterval(interval)
  }, [adapter?.status, refreshAdapter]) // eslint-disable-line react-hooks/exhaustive-deps

  const syncLIDs = async () => {
    setSyncLIDsLoading(true)
    setSyncLIDsResult(null)
    try {
      const r = await gatewayAPI.syncLIDs(channelId) as { synced?: number }
      setSyncLIDsResult(`${r.synced ?? 0} LID mappings synced to database`)
    } catch (e) {
      setSyncLIDsResult('Sync failed: ' + (e as Error).message)
    } finally {
      setSyncLIDsLoading(false)
    }
  }

  const syncContacts = async () => {
    setSyncContactsLoading(true)
    setSyncContactsResult(null)
    try {
      const r = await gatewayAPI.syncContacts(channelId) as { created?: number; updated?: number }
      setSyncContactsResult(`${r.created ?? 0} created, ${r.updated ?? 0} LIDs updated`)
      loadContacts(contactPage, contactSearch)
    } catch (e) {
      setSyncContactsResult('Sync failed: ' + (e as Error).message)
    } finally {
      setSyncContactsLoading(false)
    }
  }

  const updateSelfChat = async (enabled: boolean) => {
    await updateChannelConfig({ self_chat_enabled: enabled }, true)
  }

  const updateChannelConfig = async (configPatch: Record<string, unknown>, refreshStatus = false) => {
    if (!channel) return
    setError('')
    const updated = await gatewayAPI.updateChannel(channel.id, {
      config: { ...channel.config, ...configPatch },
    }).catch((e: Error) => { setError(e.message); return null })
    if (updated) {
      setChannel(updated as GatewayChannel)
      if (refreshStatus) refreshAdapter()
    }
  }

  const approve = async (id: string) => {
    await gatewayAPI.approvePairing(id).catch(() => {})
    load()
  }

  const reject = async (id: string) => {
    await gatewayAPI.rejectPairing(id).catch(() => {})
    load()
  }

  const approveEscalation = async (id: string) => {
    await gatewayAPI.approveEscalation(id).catch((e: Error) => setError(e.message))
    load()
  }

  const rejectEscalation = async (id: string) => {
    await gatewayAPI.rejectEscalation(id).catch((e: Error) => setError(e.message))
    load()
  }

  const addContact = async () => {
    if (!contactName.trim() || !contactPhone.trim()) {
      setError('Contact name and WhatsApp number are required')
      return
    }
    await gatewayAPI.createContact({
      channel_id: channelId,
      account_id: channel?.config?.account_id || 'default',
      display_name: contactName,
      alias: contactName.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''),
      phone_number: contactPhone,
      role: contactRole,
      auto_reply_enabled: true,
      ...(contactAgentId ? { agent_id: contactAgentId } : {}),
    }).catch((e: Error) => setError(e.message))
    setContactName('')
    setContactPhone('')
    setContactRole('trusted')
    setContactAgentId('')
    loadContacts(contactPage, contactSearch)
  }

  const deleteContact = async (id: string) => {
    if (!confirm('Delete this gateway contact?')) return
    await gatewayAPI.deleteContact(id).catch(() => {})
    loadContacts(contactPage, contactSearch)
  }

  const scheduleMessage = async () => {
    if (!schedMsg.trim() || !schedSendAt) { setError('Message and send time are required'); return }
    const body: Record<string, unknown> = {
      channel_id: channelId,
      message: schedMsg,
      send_at: new Date(schedSendAt).toISOString(),
      ...(schedContact ? { contact_id: schedContact } : {}),
    }
    if (schedFreq) {
      const rule: Record<string, unknown> = { frequency: schedFreq }
      if (schedInterval > 1) rule.interval = schedInterval
      if (schedEndAt) rule.end_at = new Date(schedEndAt).toISOString()
      if (schedMaxOcc) rule.max_occurrences = parseInt(schedMaxOcc)
      body.recurrence_rule = rule
    }
    if (schedUseAgent) body.use_agent = true
    await gatewayAPI.createScheduledMessage(body).catch((e: Error) => { setError(e.message); return null })
    setSchedMsg('')
    setSchedContact('')
    setSchedSendAt('')
    setSchedFreq('')
    setSchedInterval(1)
    setSchedEndAt('')
    setSchedMaxOcc('')
    setSchedUseAgent(false)
    load()
  }

  const cancelScheduledMessage = async (id: string) => {
    await gatewayAPI.deleteScheduledMessage(id).catch(() => {})
    load()
  }

  const sendTestMessage = async () => {
    if (!testInput.trim() || !channel) return
    setTestLoading(true)
    setTestResponse(null)
    try {
      const body: Record<string, string> = { input: testInput }
      if (testSessionId.trim()) body.session_id = testSessionId.trim()
      const res = await fetch(`${API_URL}/gateway/http/${channel.id}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      const data = await res.json()
      setTestResponse(JSON.stringify(data, null, 2))
    } catch (e) {
      setTestResponse((e as Error).message)
    } finally {
      setTestLoading(false)
    }
  }

  if (!channel) return <div className="p-6 text-sm text-gray-400 dark:text-gray-500">{error || 'Loading…'}</div>

  const isHTTP = channel.channel_type === 'http'
  const tabs = isHTTP ? HTTP_TABS : WA_TABS

  return (
    <div className="p-4 sm:p-6">
      <div className="flex items-center gap-2 text-[12px] text-gray-400 dark:text-gray-500 mb-5">
        <Link href="/gateway/channels" className="hover:text-gray-600 dark:hover:text-gray-300">Gateway</Link>
        <ChevronRight className="w-3 h-3" />
        <span className="text-gray-700 dark:text-gray-300 font-medium">{channel.name}</span>
      </div>
      <div className="flex flex-wrap items-start gap-3 justify-between mb-5">
        <div>
          <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">{channel.name}</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">{channel.description || 'Channel runtime and delivery controls.'}</p>
        </div>
        <div className="flex gap-2">
          <button onClick={openEdit} className="inline-flex items-center gap-2 px-3 py-1.5 border border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400 text-[12px] rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800">
            <Pencil className="w-3.5 h-3.5" /> Edit
          </button>
          <button onClick={load} className="inline-flex items-center gap-2 px-3 py-1.5 border border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400 text-[12px] rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800">
            <RefreshCw className="w-3.5 h-3.5" /> Refresh
          </button>
        </div>
      </div>
      {error && <div className="mb-4 text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg px-3 py-2">{error}</div>}

      {editMode && (
        <div className="mb-5 rounded-xl border border-purple-200 bg-purple-50 dark:bg-purple-500/10 p-4">
          <div className="flex items-center justify-between mb-4">
            <p className="text-sm font-semibold text-purple-900">Edit Channel Settings</p>
            <button onClick={() => setEditMode(false)} className="p-1 rounded hover:bg-purple-100 text-purple-400"><X className="w-4 h-4" /></button>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-[11px] font-medium text-gray-600 dark:text-gray-400 mb-1">Name</label>
              <input value={editName} onChange={e => setEditName(e.target.value)} className="w-full text-sm px-3 py-1.5 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900" />
            </div>
            <div>
              <label className="block text-[11px] font-medium text-gray-600 dark:text-gray-400 mb-1">Description</label>
              <input value={editDesc} onChange={e => setEditDesc(e.target.value)} className="w-full text-sm px-3 py-1.5 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900" />
            </div>
            <div>
              <label className="block text-[11px] font-medium text-gray-600 dark:text-gray-400 mb-1">Agent</label>
              <select value={editAgentId} onChange={e => setEditAgentId(e.target.value)} className="w-full text-sm px-3 py-1.5 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900">
                {agents.map(a => <option key={a.id} value={a.id}>{a.name}</option>)}
              </select>
            </div>
            <div className="flex items-center gap-3 pt-4">
              <label className="text-[11px] font-medium text-gray-600 dark:text-gray-400">Active</label>
              <button
                onClick={() => setEditIsActive(v => !v)}
                className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${editIsActive ? 'bg-green-500' : 'bg-gray-200'}`}
              >
                <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white dark:bg-gray-900 shadow transition-transform ${editIsActive ? 'translate-x-4' : 'translate-x-1'}`} />
              </button>
            </div>
            {channel.channel_type === 'whatsapp' && (
              <>
                <div>
                  <label className="block text-[11px] font-medium text-gray-600 dark:text-gray-400 mb-1">DM Policy</label>
                  <select value={editDmPolicy} onChange={e => setEditDmPolicy(e.target.value)} className="w-full text-sm px-3 py-1.5 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900">
                    <option value="">— keep existing —</option>
                    <option value="pairing">pairing</option>
                    <option value="allowlist">allowlist</option>
                    <option value="open">open</option>
                    <option value="disabled">disabled</option>
                  </select>
                </div>
                <div>
                  <label className="block text-[11px] font-medium text-gray-600 dark:text-gray-400 mb-1">Group Policy</label>
                  <select value={editGroupPolicy} onChange={e => setEditGroupPolicy(e.target.value)} className="w-full text-sm px-3 py-1.5 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900">
                    <option value="">— keep existing —</option>
                    <option value="disabled">disabled</option>
                    <option value="allowlist">allowlist</option>
                    <option value="open">open</option>
                  </select>
                </div>
                <div>
                  <label className="block text-[11px] font-medium text-gray-600 dark:text-gray-400 mb-1">History Limit</label>
                  <input type="number" value={editHistoryLimit} onChange={e => setEditHistoryLimit(e.target.value)} placeholder="e.g. 20" className="w-full text-sm px-3 py-1.5 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900" />
                </div>
                <div className="flex items-center gap-3 pt-4">
                  <label className="text-[11px] font-medium text-gray-600 dark:text-gray-400">Chat Approvals</label>
                  <button
                    onClick={() => setEditChatApprovals(v => !v)}
                    className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${editChatApprovals ? 'bg-green-500' : 'bg-gray-200'}`}
                  >
                    <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white dark:bg-gray-900 shadow transition-transform ${editChatApprovals ? 'translate-x-4' : 'translate-x-1'}`} />
                  </button>
                </div>
                <div>
                  <label className="block text-[11px] font-medium text-gray-600 dark:text-gray-400 mb-1">Adapter URL</label>
                  <input value={editAdapterUrl} onChange={e => setEditAdapterUrl(e.target.value)} placeholder="http://localhost:3001" className="w-full text-sm px-3 py-1.5 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900" />
                </div>
                <div>
                  <label className="block text-[11px] font-medium text-gray-600 dark:text-gray-400 mb-1">Device name</label>
                  <input value={editBrowserName} onChange={e => setEditBrowserName(e.target.value)} placeholder="Agent Nexus" className="w-full text-sm px-3 py-1.5 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900" />
                  <p className="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5">Shown in WhatsApp → Linked Devices. Re-link to apply.</p>
                </div>
              </>
            )}
          </div>
          <div className="flex gap-2 mt-4">
            <button onClick={saveEdit} disabled={editSaving || !editName.trim()} className="px-4 py-1.5 bg-purple-600 text-white text-[12px] font-medium rounded-lg hover:bg-purple-700 disabled:opacity-50">
              {editSaving ? 'Saving…' : 'Save changes'}
            </button>
            <button onClick={() => setEditMode(false)} className="px-4 py-1.5 border border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400 text-[12px] font-medium rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800">Cancel</button>
          </div>
        </div>
      )}
      <div className="flex border-b border-gray-100 dark:border-gray-800 mb-5 bg-gray-50 dark:bg-gray-800/60 rounded-t-lg overflow-x-auto">
        {tabs.map((t) => (
          <button key={t} onClick={() => setTab(t as Tab)} className={`px-4 py-2 text-[12px] whitespace-nowrap ${tab === t ? 'bg-white dark:bg-gray-900 text-purple-600 dark:text-purple-300 font-medium border-b-2 border-purple-500' : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'}`}>{t}</button>
        ))}
      </div>

      {tab === 'Overview' && !isHTTP && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <Info label="Type" value={channel.channel_type} />
          <Info label="Status" value={channel.is_active ? 'active' : 'disabled'} />
          <Info label="Agent" value={channel.agent_name || channel.agent_id} />
          <Info label="Account" value={channel.config?.account_id || 'default'} />
          <div className="sm:col-span-1"><Info label="Device name" value={channel.config?.browser_name || 'Agent Nexus'} /></div>
          <ToggleCard
            title="Assistant replies"
            description="Controls whether inbound messages can run the agent. The webhook still stays online for chat commands."
            checked={channel.config?.assistant_enabled !== false}
            onChange={(checked) => updateChannelConfig({ assistant_enabled: checked })}
          />
          <ToggleCard
            title="Bot mode"
            description="Silently approves unknown senders for this channel. Keep off when only selected contacts should reach the agent."
            checked={!!channel.config?.bot_mode_enabled}
            onChange={(checked) => updateChannelConfig({ bot_mode_enabled: checked })}
          />
          <div className="col-span-1 sm:col-span-2 rounded-lg border border-gray-200 dark:border-gray-700 p-4 bg-white dark:bg-gray-900">
            <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300 mb-2">Ingress URL</p>
            <div className="flex items-center gap-1">
              <code className="text-xs text-gray-500 dark:text-gray-400 font-mono bg-gray-50 dark:bg-gray-800/60 px-2 py-1 rounded truncate">{ingressURL}</code>
              <CopyButton text={ingressURL} />
            </div>
          </div>
          <div className="col-span-1 sm:col-span-2 rounded-lg border border-gray-200 dark:border-gray-700 p-4 bg-white dark:bg-gray-900">
            <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300 mb-1">Contact LID Sync</p>
            <p className="text-[11px] text-gray-400 dark:text-gray-500 mb-3">Push in-memory LID→phone mappings to the database so the pairing policy can match @lid senders to existing contacts.</p>
            <div className="flex items-center gap-3">
              <button onClick={syncLIDs} disabled={syncLIDsLoading} className="px-3 py-1.5 bg-purple-600 text-white text-[12px] font-medium rounded-lg hover:bg-purple-700 disabled:opacity-50">
                {syncLIDsLoading ? 'Syncing…' : 'Sync LIDs'}
              </button>
              {syncLIDsResult && <span className="text-[11px] text-gray-500 dark:text-gray-400">{syncLIDsResult}</span>}
            </div>
          </div>
        </div>
      )}

      {tab === 'Overview' && isHTTP && (
        <div className="space-y-4">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <Info label="Type" value="HTTP" />
            <Info label="Status" value={channel.is_active ? 'active' : 'disabled'} />
            <Info label="Agent" value={channel.agent_name || channel.agent_id} />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <ToggleCard
              title="Assistant replies"
              description="Controls whether inbound POST requests trigger an agent run."
              checked={channel.config?.assistant_enabled !== false}
              onChange={(checked) => updateChannelConfig({ assistant_enabled: checked })}
            />
            <ToggleCard
              title="Bot mode"
              description="Silently approves unknown session IDs. Keep off to require explicit session allowlisting."
              checked={!!channel.config?.bot_mode_enabled}
              onChange={(checked) => updateChannelConfig({ bot_mode_enabled: checked })}
            />
          </div>
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 p-4 bg-white dark:bg-gray-900">
            <div className="flex items-center gap-2 mb-2">
              <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-purple-50 dark:bg-purple-500/10 text-purple-700 dark:text-purple-300 border border-purple-100">POST</span>
              <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300">Inbound Webhook URL</p>
            </div>
            <div className="flex items-center gap-1">
              <code className="text-xs text-gray-500 dark:text-gray-400 font-mono bg-gray-50 dark:bg-gray-800/60 px-2 py-1 rounded truncate flex-1">{ingressURL}</code>
              <CopyButton text={ingressURL} />
            </div>
          </div>
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4 space-y-3">
            <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300">Request format</p>
            <pre className="text-xs bg-gray-50 dark:bg-gray-800/60 rounded p-3 overflow-x-auto text-gray-600 dark:text-gray-400"><code>{`POST /gateway/http/${channel.id}
Content-Type: application/json

{
  "input": "your message here",
  "session_id": "optional-session-id"
}`}</code></pre>
            <p className="text-[11px] text-gray-500 dark:text-gray-400"><code className="bg-gray-100 dark:bg-gray-800 px-1 rounded">session_id</code> is optional — the same value groups requests into one conversation thread. Omit it to start a fresh conversation on every call.</p>
            <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300 pt-1">Response — 202 Accepted</p>
            <pre className="text-xs bg-gray-50 dark:bg-gray-800/60 rounded p-3 overflow-x-auto text-gray-600 dark:text-gray-400"><code>{`{
  "run_id": "...",
  "session_id": "...",
  "conversation_id": "...",
  "status": "running"
}`}</code></pre>
          </div>
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4 space-y-3">
            <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300">Send a test message</p>
            <div className="flex flex-wrap gap-2">
              <input
                value={testInput}
                onChange={(e) => setTestInput(e.target.value)}
                placeholder="Message to send…"
                className="flex-1 min-w-[160px] text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg"
                onKeyDown={(e) => { if (e.key === 'Enter') sendTestMessage() }}
              />
              <input
                value={testSessionId}
                onChange={(e) => setTestSessionId(e.target.value)}
                placeholder="session_id (optional)"
                className="w-full sm:w-44 text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg"
              />
              <button
                onClick={sendTestMessage}
                disabled={testLoading || !testInput.trim()}
                className="px-4 py-2 rounded-md bg-purple-600 text-white text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {testLoading ? 'Sending…' : 'Send'}
              </button>
            </div>
            {testResponse && (
              <pre className="text-xs bg-gray-50 dark:bg-gray-800/60 rounded p-3 overflow-x-auto text-gray-600 dark:text-gray-400">{testResponse}</pre>
            )}
          </div>
        </div>
      )}

      {tab === 'QR Login' && (
        <div className="space-y-4 max-w-2xl">
          <AdapterStatusCard adapter={adapter} />
          <div className="flex flex-wrap gap-2">
            <button
              onClick={startLogin}
              disabled={loginLoading}
              className="inline-flex items-center gap-2 px-3 py-1.5 rounded-md bg-purple-600 text-sm text-white hover:bg-purple-700 disabled:opacity-60 disabled:cursor-not-allowed"
            >
              {loginLoading ? <Spinner /> : <QrCode className="w-4 h-4" />}
              {loginLoading ? 'Connecting…' : 'Start login'}
            </button>
            <button
              onClick={refreshAdapter}
              className="inline-flex items-center gap-2 px-3 py-1.5 rounded-md border border-gray-200 dark:border-gray-700 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800"
            >
              <RefreshCw className="w-4 h-4" /> Refresh status
            </button>
            <button
              onClick={async () => { await gatewayAPI.logout(channelId).catch(() => {}); setQR(null); await refreshAdapter() }}
              className="inline-flex items-center gap-2 px-3 py-1.5 rounded-md border border-gray-200 dark:border-gray-700 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800"
            >
              <LogOut className="w-4 h-4" /> Logout
            </button>
          </div>
          <QRPanel value={qr} loading={loginLoading && !qr} />
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4 space-y-3">
            <p className="text-sm font-medium text-gray-800 dark:text-gray-200">Link via phone number (no QR)</p>
            <p className="text-xs text-gray-500 dark:text-gray-400">Instead of scanning a QR code, get an 8-digit code and enter it on your phone: WhatsApp → Linked Devices → Link a Device → Link with phone number instead.</p>
            <div className="flex flex-wrap gap-2 items-center">
              <input
                value={pairingPhone}
                onChange={(e) => setPairingPhone(e.target.value)}
                placeholder="e.g. 917599223966 (no + or spaces)"
                className="flex-1 min-w-[200px] text-sm border border-gray-200 dark:border-gray-700 rounded-md px-3 py-1.5 focus:outline-none focus:ring-1 focus:ring-purple-400"
              />
              <button
                onClick={getPairingCode}
                disabled={pairingLoading || !pairingPhone.trim()}
                className="inline-flex items-center gap-2 px-3 py-1.5 rounded-md bg-purple-600 text-sm text-white hover:bg-purple-700 disabled:opacity-60 disabled:cursor-not-allowed whitespace-nowrap"
              >
                {pairingLoading ? <Spinner /> : null}
                {pairingLoading ? 'Getting code…' : 'Get code'}
              </button>
            </div>
            {pairingCode && (
              <div className="flex items-center gap-2 bg-gray-50 dark:bg-gray-800/60 border border-gray-200 dark:border-gray-700 rounded-md px-3 py-2">
                <span className="font-mono text-lg font-semibold tracking-widest text-purple-700 dark:text-purple-300">{pairingCode}</span>
                <CopyButton text={pairingCode} />
              </div>
            )}
          </div>
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4 flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-gray-800 dark:text-gray-200">Self-chat replies</p>
              <p className="text-xs text-gray-500 dark:text-gray-400 mt-1">When enabled, messages sent from the linked number to itself will be processed by the agent.</p>
            </div>
            <label className="inline-flex items-center cursor-pointer">
              <input type="checkbox" checked={!!channel.config?.self_chat_enabled} onChange={(e) => updateSelfChat(e.target.checked)} className="sr-only peer" />
              <span className="relative w-10 h-5 rounded-full bg-gray-200 peer-checked:bg-purple-600 after:absolute after:top-0.5 after:left-0.5 after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-5" />
            </label>
          </div>
        </div>
      )}

      {tab === 'Sessions' && <Table rows={sessions} empty="No sessions yet" columns={['peer_kind', 'peer_id', 'external_sender_id', 'conversation_id', 'last_active_at']} />}
      {tab === 'Reminders' && <Table rows={reminders} empty="No reminders yet" columns={['title', 'message', 'due_at', 'status', 'created_at']} />}
      {tab === 'Scheduled' && (
        <div className="space-y-4">
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4">
            <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300 mb-3">Schedule a message</p>
            <div className="flex flex-wrap gap-3">
              <select value={schedContact} onChange={(e) => setSchedContact(e.target.value)} className="text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900 min-w-[160px]">
                <option value="">Pick contact…</option>
                {contacts.map((c) => <option key={c.id} value={c.id}>{c.display_name}</option>)}
              </select>
              <label className="flex items-center gap-2 cursor-pointer self-center">
                <input type="checkbox" checked={schedUseAgent} onChange={(e) => setSchedUseAgent(e.target.checked)} className="h-4 w-4 rounded border-gray-300 dark:border-gray-600 text-purple-600 dark:text-purple-300 focus:ring-purple-500" />
                <span className="text-[13px] text-gray-600 dark:text-gray-400">Generate with agent</span>
              </label>
              <input value={schedMsg} onChange={(e) => setSchedMsg(e.target.value)} placeholder={schedUseAgent ? 'Prompt for agent…' : 'Message…'} className="text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg flex-1 min-w-[200px]" />
              <input type="datetime-local" value={schedSendAt} onChange={(e) => setSchedSendAt(e.target.value)} className="text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg" />
              <select value={schedFreq} onChange={(e) => setSchedFreq(e.target.value)} className="text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900">
                <option value="">One-time</option>
                <option value="daily">Daily</option>
                <option value="weekdays">Weekdays</option>
                <option value="weekly">Weekly</option>
                <option value="monthly">Monthly</option>
                <option value="yearly">Yearly</option>
              </select>
              {schedFreq && <>
                <input type="number" min={1} value={schedInterval} onChange={(e) => setSchedInterval(parseInt(e.target.value) || 1)} title="Every N periods" className="text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg w-20" placeholder="×N" />
                <input type="date" value={schedEndAt} onChange={(e) => setSchedEndAt(e.target.value)} title="End date (optional)" className="text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg" />
                <input type="number" min={1} value={schedMaxOcc} onChange={(e) => setSchedMaxOcc(e.target.value)} placeholder="Max sends" title="Max occurrences (optional)" className="text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg w-28" />
              </>}
              <button onClick={scheduleMessage} className="px-4 py-2 rounded-md bg-purple-600 text-white text-sm font-medium shrink-0">Schedule</button>
            </div>
          </div>
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden bg-white dark:bg-gray-900">
            {scheduledMessages.length === 0 ? <div className="p-8 text-center text-sm text-gray-400 dark:text-gray-500">No scheduled messages yet</div> : (
              <div className="overflow-x-auto"><table className="w-full text-sm min-w-[640px]">
                <thead><tr className="border-b border-gray-100 dark:border-gray-800 bg-gray-50 dark:bg-gray-800/60 text-left text-[11px] text-gray-500 dark:text-gray-400 uppercase tracking-wide">
                  <th className="px-4 py-2">Contact / Recipient</th>
                  <th className="px-4 py-2">Message</th>
                  <th className="px-4 py-2">Send at</th>
                  <th className="px-4 py-2">Recurrence</th>
                  <th className="px-4 py-2">Fires</th>
                  <th className="px-4 py-2">Status</th>
                  <th className="px-4 py-2" />
                </tr></thead>
                <tbody>
                  {scheduledMessages.map((m) => {
                    const contact = contacts.find((c) => c.id === m.contact_id)
                    const statusColor = m.status === 'pending' ? 'bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300' : m.status === 'sent' ? 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : m.status === 'failed' ? 'bg-red-50 dark:bg-red-500/10 text-red-700 dark:text-red-300' : 'bg-gray-50 dark:bg-gray-800/60 text-gray-500 dark:text-gray-400'
                    const recLabel = m.recurrence_rule
                      ? `${m.recurrence_rule.interval && m.recurrence_rule.interval > 1 ? `Every ${m.recurrence_rule.interval} ` : ''}${m.recurrence_rule.frequency}${m.recurrence_rule.max_occurrences ? ` (${m.recurrence_rule.max_occurrences}×)` : ''}`
                      : '—'
                    return (
                      <tr key={m.id} className="border-b border-gray-100 dark:border-gray-800 last:border-0 hover:bg-gray-50 dark:hover:bg-gray-800">
                        <td className="px-4 py-3 text-gray-700 dark:text-gray-300">{contact?.display_name || m.peer_id}</td>
                        <td className="px-4 py-3 text-gray-600 dark:text-gray-400 max-w-xs truncate" title={m.message}>
                          {m.use_agent && <span className="mr-1.5 inline-block text-[10px] px-1.5 py-0.5 rounded-full bg-purple-50 dark:bg-purple-500/10 text-purple-600 dark:text-purple-300 font-medium">agent</span>}
                          {m.message}
                        </td>
                        <td className="px-4 py-3 text-gray-500 dark:text-gray-400 whitespace-nowrap">{new Date(m.send_at).toLocaleString(undefined, { dateStyle: 'short', timeStyle: 'short' })}</td>
                        <td className="px-4 py-3 text-gray-500 dark:text-gray-400">{recLabel}</td>
                        <td className="px-4 py-3 text-gray-500 dark:text-gray-400">{m.occurrence_count}</td>
                        <td className="px-4 py-3"><span className={`text-[10px] px-1.5 py-0.5 rounded-full ${statusColor}`}>{m.status}</span></td>
                        <td className="px-4 py-3">
                          {m.status === 'pending' && <button onClick={() => cancelScheduledMessage(m.id)} className="text-[11px] px-2 py-1 rounded bg-gray-50 dark:bg-gray-800/60 text-gray-400 dark:text-gray-500 hover:bg-red-50 hover:text-red-600">Cancel</button>}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table></div>
            )}
          </div>
        </div>
      )}
      {tab === 'Escalations' && (
        <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden bg-white dark:bg-gray-900">
          {escalations.length === 0 ? <div className="p-8 text-center text-sm text-gray-400 dark:text-gray-500">No escalations yet</div> : escalations.map((e) => (
            <div key={e.id} className="px-4 py-3 border-b border-gray-100 dark:border-gray-800 last:border-b-0 flex items-center justify-between gap-4">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-gray-800 dark:text-gray-200">{e.action_type || 'WhatsApp action'}</span>
                  <span className={`text-[10px] px-1.5 py-0.5 rounded-full ${e.status === 'pending' ? 'bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300' : e.status === 'approved' ? 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : 'bg-red-50 dark:bg-red-500/10 text-red-700 dark:text-red-300'}`}>{e.status}</span>
                  {e.approval_code && <code className="text-[11px] px-1.5 py-0.5 rounded bg-gray-50 dark:bg-gray-800/60 text-gray-500 dark:text-gray-400">{e.approval_code}</code>}
                </div>
                <div className="text-xs text-gray-400 dark:text-gray-500 mt-1">
                  {e.recipient ? `Recipient ${e.recipient} · ` : ''}{e.reason || 'No reason provided'} · {new Date(e.created_at).toLocaleString()}
                </div>
                {e.message && <div className="text-xs text-gray-500 dark:text-gray-400 mt-1 truncate max-w-3xl">{e.message}</div>}
                {e.resolved_at && <div className="text-[11px] text-gray-400 dark:text-gray-500 mt-1">Resolved {new Date(e.resolved_at).toLocaleString()}{e.resolved_by_sender_id ? ` by ${e.resolved_by_sender_id}` : ''}</div>}
              </div>
              {e.status === 'pending' && (
                <div className="flex gap-2 shrink-0">
                  <button onClick={() => approveEscalation(e.id)} className="px-3 py-1 text-xs rounded bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-300">Approve</button>
                  <button onClick={() => rejectEscalation(e.id)} className="px-3 py-1 text-xs rounded bg-red-50 dark:bg-red-500/10 text-red-700 dark:text-red-300">Reject</button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
      {tab === 'Events' && <Table rows={events} empty="No events yet" columns={['event_type', 'session_id', 'run_id', 'provider_message_id', 'created_at']} />}
      {tab === 'Outbox' && <Table rows={outbox} empty="No outbound messages yet" columns={['peer_id', 'status', 'attempts', 'last_error', 'created_at']} />}
      {tab === 'Contacts' && (
        <div className="space-y-4">
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4">
            <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300 mb-3">Add WhatsApp contact</p>
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-[1fr_1fr_130px_160px_auto] gap-3">
              <input value={contactName} onChange={(e) => setContactName(e.target.value)} placeholder="Aayushi" className="text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg" />
              <input value={contactPhone} onChange={(e) => setContactPhone(e.target.value)} placeholder="+91..." className="text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg" />
              <select value={contactRole} onChange={(e) => setContactRole(e.target.value as 'owner' | 'trusted' | 'blocked')} className="text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900">
                <option value="trusted">Trusted</option>
                <option value="owner">Owner</option>
                <option value="blocked">Blocked</option>
              </select>
              <select value={contactAgentId} onChange={(e) => setContactAgentId(e.target.value)} className="text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900">
                <option value="">Channel default</option>
                {agents.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
              </select>
              <button onClick={addContact} className="px-4 py-2 rounded-md bg-purple-600 text-white text-sm font-medium">Add</button>
            </div>
          </div>
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4 flex flex-wrap items-center justify-between gap-4">
            <div>
              <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300">Sync contacts from WhatsApp</p>
              <p className="text-[11px] text-gray-400 dark:text-gray-500 mt-0.5">Import all phone contacts from the connected device. New contacts are added with auto-reply off; existing contacts get their LID updated.</p>
              {syncContactsResult && <p className="text-[11px] text-emerald-600 dark:text-emerald-300 mt-1">{syncContactsResult}</p>}
            </div>
            <button onClick={syncContacts} disabled={syncContactsLoading} className="shrink-0 px-3 py-1.5 bg-purple-600 text-white text-[12px] font-medium rounded-lg hover:bg-purple-700 disabled:opacity-50">
              {syncContactsLoading ? 'Syncing…' : 'Sync Contacts'}
            </button>
          </div>
          <div className="flex items-center gap-2">
            <input
              value={contactSearchInput}
              onChange={(e) => setContactSearchInput(e.target.value)}
              placeholder="Search by name, phone, or alias…"
              className="flex-1 text-[13px] px-3 py-2 border border-gray-200 dark:border-gray-700 rounded-lg"
            />
            {contactSearchInput && (
              <button onClick={() => { setContactSearchInput(''); setContactSearch(''); setContactPage(1) }} className="px-3 py-2 text-[12px] text-gray-500 dark:text-gray-400 border border-gray-200 dark:border-gray-700 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800">Clear</button>
            )}
            <span className="text-[12px] text-gray-400 dark:text-gray-500 shrink-0">{contactTotal} contact{contactTotal !== 1 ? 's' : ''}</span>
          </div>
          <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden bg-white dark:bg-gray-900">
            {contacts.length === 0 ? <div className="p-8 text-center text-sm text-gray-400 dark:text-gray-500">{contactSearch ? 'No contacts match your search' : 'No contacts yet'}</div> : contacts.map((c) => (
              <div key={c.id} className="px-4 py-3 border-b border-gray-100 dark:border-gray-800 last:border-b-0 flex items-center justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="text-sm font-medium text-gray-800 dark:text-gray-200">{c.display_name}</span>
                    <span className={`text-[10px] px-1.5 py-0.5 rounded-full ${c.role === 'owner' ? 'bg-purple-50 dark:bg-purple-500/10 text-purple-700 dark:text-purple-300' : c.role === 'blocked' ? 'bg-red-50 dark:bg-red-500/10 text-red-700 dark:text-red-300' : 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'}`}>{c.role}</span>
                    {!c.auto_reply_enabled && <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300">auto-reply off</span>}
                  </div>
                  <div className="flex items-center gap-3 mt-1">
                    <span className="text-xs text-gray-400 dark:text-gray-500">{c.phone_number || c.whatsapp_jid} · {c.alias || 'no alias'} · last matched {c.last_matched_at ? new Date(c.last_matched_at).toLocaleString() : 'never'}</span>
                  {c.whatsapp_lid && <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-gray-50 dark:bg-gray-800/60 text-gray-400 dark:text-gray-500 border border-gray-100 dark:border-gray-800" title="WhatsApp LID (Linked Device ID)">{c.whatsapp_lid.split('@')[0]}</span>}
                    <select
                      value={c.agent_id ?? ''}
                      onChange={async (e) => {
                        await gatewayAPI.updateContact(c.id, { ...c, agent_id: e.target.value || undefined }).catch(() => {})
                        load()
                      }}
                      className="text-[11px] px-1.5 py-0.5 border border-gray-200 dark:border-gray-700 rounded bg-white dark:bg-gray-900 text-gray-600 dark:text-gray-400 cursor-pointer"
                    >
                      <option value="">Channel default</option>
                      {agents.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
                    </select>
                  </div>
                </div>
                <div className="flex items-center gap-3 shrink-0">
                  <label className="inline-flex items-center gap-1.5 cursor-pointer" title={c.auto_reply_enabled ? 'Auto-reply on — click to disable' : 'Auto-reply off — click to enable'}>
                    <span className="text-[11px] text-gray-400 dark:text-gray-500">{c.auto_reply_enabled ? 'on' : 'off'}</span>
                    <input
                      type="checkbox"
                      checked={!!c.auto_reply_enabled}
                      onChange={async (e) => {
                        await gatewayAPI.updateContact(c.id, { ...c, auto_reply_enabled: e.target.checked }).catch(() => {})
                        load()
                      }}
                      className="sr-only peer"
                    />
                    <span className="relative w-8 h-4 rounded-full bg-gray-200 peer-checked:bg-purple-600 after:absolute after:top-0.5 after:left-0.5 after:h-3 after:w-3 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-4" />
                  </label>
                  <button onClick={() => deleteContact(c.id)} className="px-3 py-1 text-xs rounded bg-gray-50 dark:bg-gray-800/60 text-gray-500 dark:text-gray-400 hover:bg-red-50 hover:text-red-600">Delete</button>
                </div>
              </div>
            ))}
          </div>
          {contactTotal > 20 && (
            <div className="flex items-center justify-between">
              <button
                onClick={() => setContactPage((p) => Math.max(1, p - 1))}
                disabled={contactPage === 1}
                className="px-3 py-1.5 text-[12px] border border-gray-200 dark:border-gray-700 rounded-lg text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-800 disabled:opacity-40 disabled:cursor-not-allowed"
              >← Prev</button>
              <span className="text-[12px] text-gray-400 dark:text-gray-500">
                Page {contactPage} of {Math.ceil(contactTotal / 20)}
              </span>
              <button
                onClick={() => setContactPage((p) => p + 1)}
                disabled={contactPage >= Math.ceil(contactTotal / 20)}
                className="px-3 py-1.5 text-[12px] border border-gray-200 dark:border-gray-700 rounded-lg text-gray-600 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-800 disabled:opacity-40 disabled:cursor-not-allowed"
              >Next →</button>
            </div>
          )}
        </div>
      )}
      {tab === 'Pairing' && (
        <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-hidden bg-white dark:bg-gray-900">
          {pairings.length === 0 ? <div className="p-8 text-center text-sm text-gray-400 dark:text-gray-500">No pairing requests yet</div> : pairings.map((p) => (
            <div key={p.id} className="px-4 py-3 border-b border-gray-100 dark:border-gray-800 last:border-b-0 flex items-center justify-between">
              <div>
                <div className="text-sm font-medium text-gray-800 dark:text-gray-200">{p.sender_id}</div>
                <div className="text-xs text-gray-400 dark:text-gray-500">Code {p.code} · {p.status} · expires {new Date(p.expires_at).toLocaleString()}</div>
              </div>
              {p.status === 'pending' && (
                <div className="flex gap-2">
                  <button onClick={() => approve(p.id)} className="px-3 py-1 text-xs rounded bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-300">Approve</button>
                  <button onClick={() => reject(p.id)} className="px-3 py-1 text-xs rounded bg-red-50 dark:bg-red-500/10 text-red-700 dark:text-red-300">Reject</button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function Info({ label, value }: { label: string; value: string }) {
  return <div className="rounded-lg border border-gray-200 dark:border-gray-700 p-4 bg-white dark:bg-gray-900"><p className="text-[12px] text-gray-400 dark:text-gray-500 mb-1">{label}</p><p className="text-sm font-medium text-gray-800 dark:text-gray-200">{value}</p></div>
}

function ToggleCard({ title, description, checked, onChange }: { title: string; description: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 p-4 bg-white dark:bg-gray-900 flex items-center justify-between gap-4">
      <div>
        <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300">{title}</p>
        <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">{description}</p>
      </div>
      <label className="inline-flex items-center cursor-pointer shrink-0">
        <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} className="sr-only peer" />
        <span className="relative w-10 h-5 rounded-full bg-gray-200 peer-checked:bg-purple-600 after:absolute after:top-0.5 after:left-0.5 after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-5" />
      </label>
    </div>
  )
}

function Spinner() {
  return (
    <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" />
    </svg>
  )
}

function AdapterStatusCard({ adapter }: { adapter: Record<string, unknown> | null }) {
  if (!adapter) {
    return (
      <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4 flex items-center gap-3">
        <div className="w-2.5 h-2.5 rounded-full bg-gray-300 shrink-0" />
        <div>
          <p className="text-sm font-medium text-gray-700 dark:text-gray-300">Adapter status</p>
          <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">Click &quot;Refresh status&quot; to load.</p>
        </div>
      </div>
    )
  }
  const status = adapter.status as string
  const selfId = adapter.self_id as string | undefined
  const lastError = adapter.last_error as string | undefined
  const lidMapSize = adapter.lid_map_size as number | undefined

  const dot: Record<string, string> = {
    connected: 'bg-emerald-400',
    qr: 'bg-amber-400 animate-pulse',
    connecting: 'bg-amber-400 animate-pulse',
    disconnected: 'bg-red-400',
  }
  const label: Record<string, string> = {
    connected: 'Connected',
    qr: 'Waiting for QR scan',
    connecting: 'Connecting…',
    disconnected: 'Disconnected',
  }
  const phone = selfId ? selfId.split('@')[0].split(':')[0] : null

  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4 space-y-3">
      <div className="flex items-center gap-3">
        <div className={`w-2.5 h-2.5 rounded-full shrink-0 ${dot[status] ?? 'bg-gray-300'}`} />
        <p className="text-sm font-medium text-gray-800 dark:text-gray-200">{label[status] ?? status}</p>
      </div>
      {phone && (
        <div className="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
          <span className="font-medium text-gray-700 dark:text-gray-300">Linked number:</span>
          <code className="bg-gray-50 dark:bg-gray-800/60 px-2 py-0.5 rounded text-gray-600 dark:text-gray-400">+{phone}</code>
        </div>
      )}
      {lastError && status !== 'connected' && (
        <p className="text-xs text-red-500 bg-red-50 dark:bg-red-500/10 border border-red-100 rounded px-2 py-1">{lastError}</p>
      )}
      {typeof lidMapSize === 'number' && lidMapSize > 0 && (
        <p className="text-xs text-gray-400 dark:text-gray-500">{lidMapSize} contact LID{lidMapSize !== 1 ? 's' : ''} resolved</p>
      )}
    </div>
  )
}

function QRPanel({ value, loading }: { value: Record<string, unknown> | null; loading?: boolean }) {
  const dataURL = typeof value?.qr_data_url === 'string' ? value.qr_data_url : ''
  if (!dataURL && !loading) return null
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 p-4">
      <p className="text-[12px] font-medium text-gray-700 dark:text-gray-300 mb-3">Scan in WhatsApp</p>
      {loading && !dataURL ? (
        <div className="flex items-center gap-3 text-sm text-gray-500 dark:text-gray-400 py-4">
          <svg className="w-5 h-5 animate-spin text-purple-500" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" />
          </svg>
          Generating QR code…
        </div>
      ) : (
        <div className="flex items-start gap-5">
          <Image src={dataURL} alt="WhatsApp QR code" width={220} height={220} unoptimized className="border border-gray-100 dark:border-gray-800 rounded-lg" />
          <div className="text-sm text-gray-600 dark:text-gray-400 pt-1">
            <p className="font-medium text-gray-800 dark:text-gray-200 mb-2">Link this channel to WhatsApp</p>
            <ol className="text-xs text-gray-500 dark:text-gray-400 space-y-1 list-decimal list-inside">
              <li>Open WhatsApp on your phone</li>
              <li>Tap <strong>Linked Devices</strong></li>
              <li>Tap <strong>Link a device</strong></li>
              <li>Scan this QR code</li>
            </ol>
            <p className="text-xs text-gray-400 dark:text-gray-500 mt-3">Status updates automatically after scanning.</p>
          </div>
        </div>
      )}
    </div>
  )
}

const TIME_COLS = new Set(['created_at', 'updated_at', 'due_at', 'last_active_at', 'last_matched_at', 'resolved_at', 'expires_at', 'received_at', 'completed_at', 'started_at'])

function fmtCell(col: string, val: unknown): string {
  if (val == null || val === '') return '—'
  if (TIME_COLS.has(col) && typeof val === 'string' && val.length > 0) {
    const d = new Date(val)
    if (!isNaN(d.getTime())) return d.toLocaleString(undefined, { dateStyle: 'short', timeStyle: 'short' })
  }
  return String(val)
}

function Table({ rows, columns, empty }: { rows: object[]; columns: string[]; empty: string }) {
  if (rows.length === 0) return <div className="p-8 text-center text-sm text-gray-400 dark:text-gray-500 border border-gray-200 dark:border-gray-700 rounded-lg bg-white dark:bg-gray-900">{empty}</div>
  return (
    <div className="rounded-lg border border-gray-200 dark:border-gray-700 overflow-x-auto bg-white dark:bg-gray-900">
      <table className="w-full text-sm">
        <thead className="bg-gray-50 dark:bg-gray-800/60 text-xs text-gray-500 dark:text-gray-400">
          <tr>{columns.map((c) => <th key={c} className="text-left font-medium px-3 py-2">{c.replaceAll('_', ' ')}</th>)}</tr>
        </thead>
        <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
          {rows.map((r, i) => {
            const row = r as Record<string, unknown>
            return (
            <tr key={String(row.id ?? i)}>
              {columns.map((c) => <td key={c} className="px-3 py-2 text-xs text-gray-600 dark:text-gray-400 max-w-xs truncate">{fmtCell(c, row[c])}</td>)}
            </tr>
          )})}
        </tbody>
      </table>
    </div>
  )
}
