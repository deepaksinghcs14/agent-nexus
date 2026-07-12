'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Edit2, Plus, Radio, Trash2 } from 'lucide-react'
import { gatewayAPI } from '@/lib/api'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable, Row, Cell } from '@/components/ui/DataTable'
import type { GatewayChannel } from '@/types'
import { CopyButton } from '@/components/ui/CopyButton'

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'

function ingressURL(c: GatewayChannel) {
  return `${API_URL}/gateway/${c.channel_type}/${c.id}`
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
    <div className="p-4 sm:p-6 max-w-6xl">
      <PageHeader
        eyebrow="Integrations"
        title="Gateway"
        subtitle="Persistent channel entrypoints for always-on agents (WhatsApp, HTTP)."
        actions={
          <Link href="/gateway/channels/new" className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-[10px] bg-gradient-to-br from-accent to-accent-ink hover:opacity-95 text-white text-[13px] font-semibold shadow-card">
            <Plus className="w-4 h-4" /> New channel
          </Link>
        }
      />

      {loading ? (
        <div className="py-10 text-center text-sm text-faint">Loading…</div>
      ) : channels.length === 0 ? (
        <div className="border border-dashed border-border-strong rounded-2xl py-16 text-center bg-surface">
          <Radio className="w-10 h-10 text-faint mx-auto mb-3" />
          <p className="text-sm font-medium text-muted-foreground mb-1">No gateway channels yet</p>
          <p className="text-sm text-faint mb-4">Create a WhatsApp or HTTP channel to route messages into an agent.</p>
          <Link href="/gateway/channels/new" className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-accent text-white text-sm font-medium">
            <Plus className="w-4 h-4" /> Create channel
          </Link>
        </div>
      ) : (
        <DataTable columns={['Channel', 'Type', 'Agent', 'Ingress URL', 'Active', '']} minWidth={780}>
          {channels.map((c) => (
            <Row key={c.id}>
              <Cell className="!whitespace-normal">
                <div className="font-medium text-foreground">{c.name}</div>
                {c.description && <div className="text-xs text-faint mt-0.5 line-clamp-1 max-w-xs">{c.description}</div>}
              </Cell>
              <Cell>
                <span className={`font-mono text-[10px] px-2 py-0.5 rounded-full border ${c.channel_type === 'whatsapp' ? 'border-good/40 text-good' : 'border-info/40 text-info'}`}>{c.channel_type}</span>
              </Cell>
              <Cell><span className="text-[12px] text-muted-foreground">{c.agent_name || c.agent_id}</span></Cell>
              <Cell>
                <span className="inline-flex items-center gap-1">
                  <code className="font-mono text-[11px] text-muted-foreground bg-muted px-2 py-0.5 rounded truncate max-w-[220px] inline-block align-middle">{ingressURL(c)}</code>
                  <CopyButton text={ingressURL(c)} />
                </span>
              </Cell>
              <Cell>
                <button onClick={() => toggle(c)} className={`relative inline-flex h-5 w-9 items-center rounded-full ${c.is_active ? 'bg-accent' : 'bg-muted'}`} title={c.is_active ? 'Disable' : 'Enable'}>
                  <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-surface shadow transition-transform ${c.is_active ? 'translate-x-4' : 'translate-x-1'}`} />
                </button>
              </Cell>
              <Cell className="text-right">
                <span className="inline-flex items-center gap-0.5">
                  <Link href={`/gateway/channels/${c.id}`} className="p-1.5 rounded text-faint hover:text-foreground hover:bg-muted" title="Open"><Edit2 className="w-4 h-4" /></Link>
                  <button onClick={() => remove(c.id)} className="p-1.5 rounded text-faint hover:text-crit hover:bg-crit/10" title="Delete"><Trash2 className="w-4 h-4" /></button>
                </span>
              </Cell>
            </Row>
          ))}
        </DataTable>
      )}
    </div>
  )
}
