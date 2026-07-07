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
      className="p-1 rounded hover:bg-muted text-faint hover:text-gray-600 dark:hover:text-gray-300"
      title="Copy ingress URL"
    >
      {copied ? <Check className="w-3.5 h-3.5 text-green-600 dark:text-green-300" /> : <Copy className="w-3.5 h-3.5" />}
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
    <div className="p-4 sm:p-6">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-8">
        <div>
          <h1 className="text-xl font-semibold text-foreground">Gateway Channels</h1>
          <p className="text-sm text-muted-foreground mt-1">Persistent channel entrypoints for always-on agents.</p>
        </div>
        <Link href="/gateway/channels/new" className="flex items-center gap-2 px-4 py-2 rounded-md bg-accent hover:bg-accent-hover text-white text-sm font-medium">
          <Plus className="w-4 h-4" /> New channel
        </Link>
      </div>

      {loading ? (
        <div className="py-10 text-center text-sm text-faint">Loading…</div>
      ) : channels.length === 0 ? (
        <div className="py-16 text-center">
          <Radio className="w-10 h-10 text-faint mx-auto mb-3" />
          <p className="text-sm font-medium text-muted-foreground mb-1">No gateway channels yet</p>
          <p className="text-sm text-faint mb-4">Create a WhatsApp or HTTP channel to route messages into an agent.</p>
          <Link href="/gateway/channels/new" className="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-accent text-white text-sm font-medium">
            <Plus className="w-4 h-4" /> Create channel
          </Link>
        </div>
      ) : (
        <div className="rounded-lg border border-border-strong overflow-hidden divide-y divide-border">
          {channels.map((c) => (
            <div key={c.id} className="bg-surface px-4 sm:px-5 py-4">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 mb-1">
                    <span className="text-sm font-medium text-foreground">{c.name}</span>
                    <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${c.channel_type === 'whatsapp' ? 'bg-green-50 dark:bg-green-500/10 text-green-700 dark:text-green-300' : 'bg-blue-50 dark:bg-blue-500/10 text-blue-700 dark:text-blue-300'}`}>
                      {c.channel_type}
                    </span>
                    <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${c.is_active ? 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : 'bg-muted text-muted-foreground'}`}>
                      {c.is_active ? 'active' : 'disabled'}
                    </span>
                  </div>
                  {c.description && <p className="text-xs text-faint mb-1">{c.description}</p>}
                  <p className="text-xs text-muted-foreground">Agent: {c.agent_name || c.agent_id}</p>
                  <div className="flex items-center gap-1 mt-2">
                    <code className="text-xs text-muted-foreground font-mono bg-muted px-2 py-0.5 rounded truncate max-w-[200px] sm:max-w-md">{ingressURL(c)}</code>
                    <CopyButton text={ingressURL(c)} />
                  </div>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <button onClick={() => toggle(c)} className={`relative inline-flex h-5 w-9 items-center rounded-full ${c.is_active ? 'bg-accent' : 'bg-gray-200'}`} title={c.is_active ? 'Disable' : 'Enable'}>
                    <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-surface shadow transition-transform ${c.is_active ? 'translate-x-4' : 'translate-x-1'}`} />
                  </button>
                  <Link href={`/gateway/channels/${c.id}`} className="p-1.5 rounded text-faint hover:text-gray-600 dark:hover:text-gray-300 hover:bg-muted" title="Open">
                    <Edit2 className="w-4 h-4" />
                  </Link>
                  <button onClick={() => remove(c.id)} className="p-1.5 rounded text-faint hover:text-red-500 hover:bg-red-50" title="Delete">
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
