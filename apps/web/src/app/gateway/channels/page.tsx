'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Check, Copy, Edit2, Plus, Radio, Trash2 } from 'lucide-react'
import { gatewayAPI } from '@/lib/api'
import type { GatewayChannel } from '@/types'

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'

function ingressURL(c: GatewayChannel) {
  return `${API_URL}/gateway/${c.channel_type}/${c.id}`
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      onClick={() => { navigator.clipboard.writeText(text); setCopied(true); setTimeout(() => setCopied(false), 1600) }}
      className="p-1 rounded hover:bg-gray-100 text-gray-400 hover:text-gray-600"
      title="Copy ingress URL"
    >
      {copied ? <Check className="w-3.5 h-3.5 text-green-600" /> : <Copy className="w-3.5 h-3.5" />}
    </button>
  )
}

export default function GatewayChannelsPage() {
  const [channels, setChannels] = useState<GatewayChannel[]>([])
  const [loading, setLoading] = useState(true)

  const load = () => {
    gatewayAPI.listChannels()
      .then((r) => setChannels(((r as { data?: GatewayChannel[] }).data) ?? []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const toggle = async (c: GatewayChannel) => {
    await gatewayAPI.updateChannel(c.id, { is_active: !c.is_active }).catch(() => {})
    load()
  }

  const remove = async (id: string) => {
    if (!confirm('Delete this gateway channel? Existing inbound traffic for this channel will stop.')) return
    await gatewayAPI.deleteChannel(id).catch(() => {})
    load()
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Gateway Channels</h1>
          <p className="text-sm text-gray-500 mt-1">Persistent channel entrypoints for always-on agents.</p>
        </div>
        <Link href="/gateway/channels/new" className="flex items-center gap-2 px-4 py-2 rounded-md bg-purple-600 hover:bg-purple-700 text-white text-sm font-medium">
          <Plus className="w-4 h-4" /> New channel
        </Link>
      </div>

      {loading ? (
        <div className="py-10 text-center text-sm text-gray-400">Loading…</div>
      ) : channels.length === 0 ? (
        <div className="py-16 text-center">
          <Radio className="w-10 h-10 text-gray-300 mx-auto mb-3" />
          <p className="text-sm font-medium text-gray-600 mb-1">No gateway channels yet</p>
          <p className="text-sm text-gray-400 mb-4">Create a WhatsApp or HTTP channel to route messages into an agent.</p>
          <Link href="/gateway/channels/new" className="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-purple-600 text-white text-sm font-medium">
            <Plus className="w-4 h-4" /> Create channel
          </Link>
        </div>
      ) : (
        <div className="rounded-lg border border-gray-200 overflow-hidden divide-y divide-gray-100">
          {channels.map((c) => (
            <div key={c.id} className="bg-white px-5 py-4">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 mb-1">
                    <span className="text-sm font-medium text-gray-900">{c.name}</span>
                    <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${c.channel_type === 'whatsapp' ? 'bg-green-50 text-green-700' : 'bg-blue-50 text-blue-700'}`}>
                      {c.channel_type}
                    </span>
                    <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${c.is_active ? 'bg-emerald-50 text-emerald-700' : 'bg-gray-100 text-gray-500'}`}>
                      {c.is_active ? 'active' : 'disabled'}
                    </span>
                  </div>
                  {c.description && <p className="text-xs text-gray-400 mb-1">{c.description}</p>}
                  <p className="text-xs text-gray-500">Agent: {c.agent_name || c.agent_id}</p>
                  <div className="flex items-center gap-1 mt-2">
                    <code className="text-xs text-gray-500 font-mono bg-gray-50 px-2 py-0.5 rounded truncate max-w-md">{ingressURL(c)}</code>
                    <CopyButton text={ingressURL(c)} />
                  </div>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <button onClick={() => toggle(c)} className={`relative inline-flex h-5 w-9 items-center rounded-full ${c.is_active ? 'bg-purple-600' : 'bg-gray-200'}`} title={c.is_active ? 'Disable' : 'Enable'}>
                    <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${c.is_active ? 'translate-x-4' : 'translate-x-1'}`} />
                  </button>
                  <Link href={`/gateway/channels/${c.id}`} className="p-1.5 rounded text-gray-400 hover:text-gray-600 hover:bg-gray-100" title="Open">
                    <Edit2 className="w-4 h-4" />
                  </Link>
                  <button onClick={() => remove(c.id)} className="p-1.5 rounded text-gray-400 hover:text-red-500 hover:bg-red-50" title="Delete">
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
