'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import Image from 'next/image'
import { Check, ChevronRight, Copy, LogOut, QrCode, RefreshCw } from 'lucide-react'
import { gatewayAPI } from '@/lib/api'
import type { ChannelSession, GatewayChannel, GatewayContact, GatewayEscalation, GatewayEvent, GatewayOutboundMessage, GatewayPairingRequest, GatewayReminder } from '@/types'

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'
const TABS = ['Overview', 'QR Login', 'Contacts', 'Sessions', 'Pairing', 'Reminders', 'Escalations', 'Events', 'Outbox'] as const
type Tab = typeof TABS[number]

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button onClick={() => { navigator.clipboard.writeText(text); setCopied(true); setTimeout(() => setCopied(false), 1600) }} className="p-1 rounded hover:bg-gray-100 text-gray-400 hover:text-gray-600" title="Copy">
      {copied ? <Check className="w-3.5 h-3.5 text-green-600" /> : <Copy className="w-3.5 h-3.5" />}
    </button>
  )
}

export default function GatewayChannelDetailPage({ params }: { params: { id: string } }) {
  const [tab, setTab] = useState<Tab>('Overview')
  const [channel, setChannel] = useState<GatewayChannel | null>(null)
  const [sessions, setSessions] = useState<ChannelSession[]>([])
  const [events, setEvents] = useState<GatewayEvent[]>([])
  const [pairings, setPairings] = useState<GatewayPairingRequest[]>([])
  const [outbox, setOutbox] = useState<GatewayOutboundMessage[]>([])
  const [contacts, setContacts] = useState<GatewayContact[]>([])
  const [reminders, setReminders] = useState<GatewayReminder[]>([])
  const [escalations, setEscalations] = useState<GatewayEscalation[]>([])
  const [adapter, setAdapter] = useState<Record<string, unknown> | null>(null)
  const [qr, setQR] = useState<Record<string, unknown> | null>(null)
  const [error, setError] = useState('')
  const [contactName, setContactName] = useState('')
  const [contactPhone, setContactPhone] = useState('')
  const [contactRole, setContactRole] = useState<'owner' | 'trusted' | 'blocked'>('trusted')

  const ingressURL = useMemo(() => channel ? `${API_URL}/gateway/${channel.channel_type}/${channel.id}` : '', [channel])

  const load = useCallback(() => {
    gatewayAPI.getChannel(params.id).then((c) => setChannel(c as GatewayChannel)).catch((e: Error) => setError(e.message))
    const q = `channel_id=${params.id}`
    gatewayAPI.listSessions(q).then((r) => setSessions(((r as { data?: ChannelSession[] }).data) ?? [])).catch(() => {})
    gatewayAPI.listEvents(q).then((r) => setEvents(((r as { data?: GatewayEvent[] }).data) ?? [])).catch(() => {})
    gatewayAPI.listPairings(q).then((r) => setPairings(((r as { data?: GatewayPairingRequest[] }).data) ?? [])).catch(() => {})
    gatewayAPI.listOutbox(q).then((r) => setOutbox(((r as { data?: GatewayOutboundMessage[] }).data) ?? [])).catch(() => {})
    gatewayAPI.listContacts(q).then((r) => setContacts(((r as { data?: GatewayContact[] }).data) ?? [])).catch(() => {})
    gatewayAPI.listReminders(q).then((r) => setReminders(((r as { data?: GatewayReminder[] }).data) ?? [])).catch(() => {})
    gatewayAPI.listEscalations(q).then((r) => setEscalations(((r as { data?: GatewayEscalation[] }).data) ?? [])).catch(() => {})
  }, [params.id])

  useEffect(() => { load() }, [load])

  const refreshAdapter = () => {
    gatewayAPI.adapterStatus(params.id).then((r) => setAdapter(r as Record<string, unknown>)).catch((e: Error) => setError(e.message))
  }

  const startLogin = async () => {
    setError('')
    await gatewayAPI.startLogin(params.id).catch((e: Error) => setError(e.message))
    const r = await gatewayAPI.getQR(params.id).catch((e: Error) => { setError(e.message); return null })
    if (r) setQR(r as Record<string, unknown>)
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
      channel_id: params.id,
      account_id: channel?.config?.account_id || 'default',
      display_name: contactName,
      alias: contactName.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''),
      phone_number: contactPhone,
      role: contactRole,
      auto_reply_enabled: true,
    }).catch((e: Error) => setError(e.message))
    setContactName('')
    setContactPhone('')
    setContactRole('trusted')
    load()
  }

  const deleteContact = async (id: string) => {
    if (!confirm('Delete this gateway contact?')) return
    await gatewayAPI.deleteContact(id).catch(() => {})
    load()
  }

  if (!channel) return <div className="p-6 text-sm text-gray-400">{error || 'Loading…'}</div>

  return (
    <div className="p-6">
      <div className="flex items-center gap-2 text-[12px] text-gray-400 mb-5">
        <Link href="/gateway/channels" className="hover:text-gray-600">Gateway</Link>
        <ChevronRight className="w-3 h-3" />
        <span className="text-gray-700 font-medium">{channel.name}</span>
      </div>
      <div className="flex items-start justify-between mb-5">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">{channel.name}</h1>
          <p className="text-sm text-gray-500 mt-1">{channel.description || 'Channel runtime and delivery controls.'}</p>
        </div>
        <button onClick={load} className="inline-flex items-center gap-2 px-3 py-1.5 border border-gray-200 text-gray-600 text-[12px] rounded-lg hover:bg-gray-50">
          <RefreshCw className="w-3.5 h-3.5" /> Refresh
        </button>
      </div>
      {error && <div className="mb-4 text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">{error}</div>}
      <div className="flex border-b border-gray-100 mb-5 bg-gray-50 rounded-t-lg overflow-hidden">
        {TABS.map((t) => (
          <button key={t} onClick={() => setTab(t)} className={`px-4 py-2 text-[12px] whitespace-nowrap ${tab === t ? 'bg-white text-purple-600 font-medium border-b-2 border-purple-500' : 'text-gray-500 hover:text-gray-700'}`}>{t}</button>
        ))}
      </div>

      {tab === 'Overview' && (
        <div className="grid grid-cols-2 gap-4">
          <Info label="Type" value={channel.channel_type} />
          <Info label="Status" value={channel.is_active ? 'active' : 'disabled'} />
          <Info label="Agent" value={channel.agent_name || channel.agent_id} />
          <Info label="Account" value={channel.config?.account_id || 'default'} />
          <ToggleCard
            title="Assistant replies"
            description="Controls whether inbound WhatsApp messages can run the agent. The webhook still stays online for chat commands."
            checked={channel.config?.assistant_enabled !== false}
            onChange={(checked) => updateChannelConfig({ assistant_enabled: checked })}
          />
          <ToggleCard
            title="Bot mode"
            description="Silently approves unknown senders for this channel. Keep off when only selected contacts should reach the agent."
            checked={!!channel.config?.bot_mode_enabled}
            onChange={(checked) => updateChannelConfig({ bot_mode_enabled: checked })}
          />
          <div className="col-span-2 rounded-lg border border-gray-200 p-4 bg-white">
            <p className="text-[12px] font-medium text-gray-700 mb-2">Ingress URL</p>
            <div className="flex items-center gap-1">
              <code className="text-xs text-gray-500 font-mono bg-gray-50 px-2 py-1 rounded truncate">{ingressURL}</code>
              <CopyButton text={ingressURL} />
            </div>
          </div>
        </div>
      )}

      {tab === 'QR Login' && (
        <div className="space-y-4">
          <div className="rounded-lg border border-gray-200 bg-white p-4 flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-gray-800">Self-chat replies</p>
              <p className="text-xs text-gray-500 mt-1">Enable this when the linked WhatsApp account should message the agent from its own number.</p>
            </div>
            <label className="inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                checked={!!channel.config?.self_chat_enabled}
                onChange={(e) => updateSelfChat(e.target.checked)}
                className="sr-only peer"
              />
              <span className="relative w-10 h-5 rounded-full bg-gray-200 peer-checked:bg-[#534AB7] after:absolute after:top-0.5 after:left-0.5 after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-5" />
            </label>
          </div>
          <div className="flex gap-2">
            <button onClick={refreshAdapter} className="inline-flex items-center gap-2 px-3 py-1.5 rounded-md border border-gray-200 text-sm text-gray-700 hover:bg-gray-50"><RefreshCw className="w-4 h-4" /> Status</button>
            <button onClick={startLogin} className="inline-flex items-center gap-2 px-3 py-1.5 rounded-md bg-[#534AB7] text-sm text-white hover:bg-[#4a42a3]"><QrCode className="w-4 h-4" /> Start login</button>
            <button onClick={() => gatewayAPI.logout(params.id).catch(() => {})} className="inline-flex items-center gap-2 px-3 py-1.5 rounded-md border border-gray-200 text-sm text-gray-700 hover:bg-gray-50"><LogOut className="w-4 h-4" /> Logout</button>
          </div>
          <JsonPanel title="Adapter status" value={adapter ?? { adapter_url: channel.config?.adapter_url, status: 'not loaded' }} />
          <QRPanel value={qr} />
        </div>
      )}

      {tab === 'Sessions' && <Table rows={sessions} empty="No sessions yet" columns={['peer_kind', 'peer_id', 'external_sender_id', 'conversation_id', 'last_active_at']} />}
      {tab === 'Reminders' && <Table rows={reminders} empty="No reminders yet" columns={['title', 'message', 'due_at', 'status', 'created_at']} />}
      {tab === 'Escalations' && (
        <div className="rounded-lg border border-gray-200 overflow-hidden bg-white">
          {escalations.length === 0 ? <div className="p-8 text-center text-sm text-gray-400">No escalations yet</div> : escalations.map((e) => (
            <div key={e.id} className="px-4 py-3 border-b border-gray-100 last:border-b-0 flex items-center justify-between gap-4">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-gray-800">{e.action_type || 'WhatsApp action'}</span>
                  <span className={`text-[10px] px-1.5 py-0.5 rounded-full ${e.status === 'pending' ? 'bg-amber-50 text-amber-700' : e.status === 'approved' ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'}`}>{e.status}</span>
                  {e.approval_code && <code className="text-[11px] px-1.5 py-0.5 rounded bg-gray-50 text-gray-500">{e.approval_code}</code>}
                </div>
                <div className="text-xs text-gray-400 mt-1">
                  {e.recipient ? `Recipient ${e.recipient} · ` : ''}{e.reason || 'No reason provided'} · {new Date(e.created_at).toLocaleString()}
                </div>
                {e.message && <div className="text-xs text-gray-500 mt-1 truncate max-w-3xl">{e.message}</div>}
                {e.resolved_at && <div className="text-[11px] text-gray-400 mt-1">Resolved {new Date(e.resolved_at).toLocaleString()}{e.resolved_by_sender_id ? ` by ${e.resolved_by_sender_id}` : ''}</div>}
              </div>
              {e.status === 'pending' && (
                <div className="flex gap-2 shrink-0">
                  <button onClick={() => approveEscalation(e.id)} className="px-3 py-1 text-xs rounded bg-emerald-50 text-emerald-700">Approve</button>
                  <button onClick={() => rejectEscalation(e.id)} className="px-3 py-1 text-xs rounded bg-red-50 text-red-700">Reject</button>
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
          <div className="rounded-lg border border-gray-200 bg-white p-4">
            <p className="text-[12px] font-medium text-gray-700 mb-3">Add WhatsApp contact</p>
            <div className="grid grid-cols-[1fr_1fr_150px_auto] gap-3">
              <input value={contactName} onChange={(e) => setContactName(e.target.value)} placeholder="Aayushi" className="text-[13px] px-3 py-2 border border-gray-200 rounded-lg" />
              <input value={contactPhone} onChange={(e) => setContactPhone(e.target.value)} placeholder="+91..." className="text-[13px] px-3 py-2 border border-gray-200 rounded-lg" />
              <select value={contactRole} onChange={(e) => setContactRole(e.target.value as 'owner' | 'trusted' | 'blocked')} className="text-[13px] px-3 py-2 border border-gray-200 rounded-lg bg-white">
                <option value="trusted">Trusted</option>
                <option value="owner">Owner</option>
                <option value="blocked">Blocked</option>
              </select>
              <button onClick={addContact} className="px-4 py-2 rounded-md bg-[#534AB7] text-white text-sm font-medium">Add</button>
            </div>
          </div>
          <div className="rounded-lg border border-gray-200 overflow-hidden bg-white">
            {contacts.length === 0 ? <div className="p-8 text-center text-sm text-gray-400">No contacts yet</div> : contacts.map((c) => (
              <div key={c.id} className="px-4 py-3 border-b border-gray-100 last:border-b-0 flex items-center justify-between">
                <div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-gray-800">{c.display_name}</span>
                    <span className={`text-[10px] px-1.5 py-0.5 rounded-full ${c.role === 'owner' ? 'bg-purple-50 text-purple-700' : c.role === 'blocked' ? 'bg-red-50 text-red-700' : 'bg-emerald-50 text-emerald-700'}`}>{c.role}</span>
                  </div>
                  <div className="text-xs text-gray-400">{c.phone_number || c.whatsapp_jid} · {c.alias || 'no alias'} · last matched {c.last_matched_at ? new Date(c.last_matched_at).toLocaleString() : 'never'}</div>
                </div>
                <button onClick={() => deleteContact(c.id)} className="px-3 py-1 text-xs rounded bg-gray-50 text-gray-500 hover:bg-red-50 hover:text-red-600">Delete</button>
              </div>
            ))}
          </div>
        </div>
      )}
      {tab === 'Pairing' && (
        <div className="rounded-lg border border-gray-200 overflow-hidden bg-white">
          {pairings.length === 0 ? <div className="p-8 text-center text-sm text-gray-400">No pairing requests yet</div> : pairings.map((p) => (
            <div key={p.id} className="px-4 py-3 border-b border-gray-100 last:border-b-0 flex items-center justify-between">
              <div>
                <div className="text-sm font-medium text-gray-800">{p.sender_id}</div>
                <div className="text-xs text-gray-400">Code {p.code} · {p.status} · expires {new Date(p.expires_at).toLocaleString()}</div>
              </div>
              {p.status === 'pending' && (
                <div className="flex gap-2">
                  <button onClick={() => approve(p.id)} className="px-3 py-1 text-xs rounded bg-emerald-50 text-emerald-700">Approve</button>
                  <button onClick={() => reject(p.id)} className="px-3 py-1 text-xs rounded bg-red-50 text-red-700">Reject</button>
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
  return <div className="rounded-lg border border-gray-200 p-4 bg-white"><p className="text-[12px] text-gray-400 mb-1">{label}</p><p className="text-sm font-medium text-gray-800">{value}</p></div>
}

function ToggleCard({ title, description, checked, onChange }: { title: string; description: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <div className="rounded-lg border border-gray-200 p-4 bg-white flex items-center justify-between gap-4">
      <div>
        <p className="text-[12px] font-medium text-gray-700">{title}</p>
        <p className="text-xs text-gray-400 mt-1">{description}</p>
      </div>
      <label className="inline-flex items-center cursor-pointer shrink-0">
        <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} className="sr-only peer" />
        <span className="relative w-10 h-5 rounded-full bg-gray-200 peer-checked:bg-[#534AB7] after:absolute after:top-0.5 after:left-0.5 after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-5" />
      </label>
    </div>
  )
}

function JsonPanel({ title, value }: { title: string; value: unknown }) {
  return <div className="rounded-lg border border-gray-200 bg-white p-4"><p className="text-[12px] font-medium text-gray-700 mb-2">{title}</p><pre className="text-xs bg-gray-50 rounded p-3 overflow-auto text-gray-600">{JSON.stringify(value, null, 2)}</pre></div>
}

function QRPanel({ value }: { value: Record<string, unknown> | null }) {
  const dataURL = typeof value?.qr_data_url === 'string' ? value.qr_data_url : ''
  const status = typeof value?.status === 'string' ? value.status : 'not requested'
  return (
    <div className="rounded-lg border border-gray-200 bg-white p-4">
      <p className="text-[12px] font-medium text-gray-700 mb-3">WhatsApp QR</p>
      {dataURL ? (
        <div className="flex items-start gap-5">
          <Image src={dataURL} alt="WhatsApp login QR code" width={256} height={256} unoptimized className="border border-gray-100 rounded-lg" />
          <div className="text-sm text-gray-600">
            <p className="font-medium text-gray-800 mb-1">Scan this QR in WhatsApp</p>
            <p className="text-xs text-gray-500">Open WhatsApp, go to linked devices, and scan this code. The status will switch to connected after login.</p>
          </div>
        </div>
      ) : (
        <div className="text-sm text-gray-500 bg-gray-50 border border-gray-100 rounded-lg p-4">
          QR not available. Current status: {status}
        </div>
      )}
    </div>
  )
}

function Table({ rows, columns, empty }: { rows: object[]; columns: string[]; empty: string }) {
  if (rows.length === 0) return <div className="p-8 text-center text-sm text-gray-400 border border-gray-200 rounded-lg bg-white">{empty}</div>
  return (
    <div className="rounded-lg border border-gray-200 overflow-x-auto bg-white">
      <table className="w-full text-sm">
        <thead className="bg-gray-50 text-xs text-gray-500">
          <tr>{columns.map((c) => <th key={c} className="text-left font-medium px-3 py-2">{c.replaceAll('_', ' ')}</th>)}</tr>
        </thead>
        <tbody className="divide-y divide-gray-100">
          {rows.map((r, i) => {
            const row = r as Record<string, unknown>
            return (
            <tr key={String(row.id ?? i)}>
              {columns.map((c) => <td key={c} className="px-3 py-2 text-xs text-gray-600 max-w-xs truncate">{String(row[c] ?? '—')}</td>)}
            </tr>
          )})}
        </tbody>
      </table>
    </div>
  )
}
