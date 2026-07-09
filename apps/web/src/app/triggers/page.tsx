'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Check, Copy, Edit2, Plus, Trash2, Zap } from 'lucide-react'
import { webhookTriggersAPI } from '@/lib/api'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable, Row, Cell } from '@/components/ui/DataTable'
import type { WebhookTrigger } from '@/types'

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'

function webhookURL(id: string) {
  return `${API_URL}/webhook/${id}`
}

function formatDate(s: string | null | undefined) {
  if (!s) return '—'
  return new Date(s).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  const handle = () => {
    navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }
  return (
    <button onClick={handle} className="p-1 rounded hover:bg-muted text-faint hover:text-foreground transition-colors" title="Copy URL">
      {copied ? <Check className="w-3.5 h-3.5 text-good" /> : <Copy className="w-3.5 h-3.5" />}
    </button>
  )
}

export default function TriggersPage() {
  const [triggers, setTriggers] = useState<WebhookTrigger[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = () => {
    webhookTriggersAPI.list()
      .then((r) => { setTriggers(((r as { data?: WebhookTrigger[] }).data) ?? []); setError('') })
      .catch((e: Error) => setError(e.message || 'Failed to load triggers'))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this webhook trigger? Any external services using this URL will stop working.')) return
    try {
      await webhookTriggersAPI.delete(id)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to delete trigger')
    }
    load()
  }

  const handleToggle = async (t: WebhookTrigger) => {
    try {
      await webhookTriggersAPI.update(t.id, { is_active: !t.is_active })
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to update trigger')
    }
    load()
  }

  return (
    <div className="p-4 sm:p-6 max-w-6xl">
      <PageHeader
        eyebrow="Integrations"
        title="Triggers"
        subtitle="Inbound HTTP events that automatically run an agent or workflow."
        actions={
          <Link href="/triggers/new" className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-[10px] bg-gradient-to-br from-accent to-accent-ink hover:opacity-95 text-white text-[13px] font-semibold shadow-card">
            <Plus className="w-4 h-4" /> New trigger
          </Link>
        }
      />

      {error && (
        <div className="mb-4 rounded-lg border border-crit/30 bg-crit/10 px-3 py-2 text-sm text-crit">{error}</div>
      )}

      {loading ? (
        <div className="py-10 text-center text-sm text-faint">Loading…</div>
      ) : triggers.length === 0 ? (
        <div className="border border-dashed border-border-strong rounded-2xl py-16 text-center bg-surface">
          <Zap className="w-10 h-10 text-faint mx-auto mb-3" />
          <p className="text-sm font-medium text-muted-foreground mb-1">No webhook triggers yet</p>
          <p className="text-sm text-faint mb-4">Create a trigger to run agents or workflows from external HTTP events.</p>
          <Link href="/triggers/new" className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-accent text-white text-sm font-medium hover:bg-accent-hover">
            <Plus className="w-4 h-4" /> Create your first trigger
          </Link>
        </div>
      ) : (
        <DataTable columns={['Trigger', 'Target', 'Webhook URL', 'Fired', 'Active', '']} minWidth={780}>
          {triggers.map((t) => {
            const url = webhookURL(t.id)
            return (
              <Row key={t.id}>
                <Cell className="!whitespace-normal">
                  <div className="font-medium text-foreground">{t.name}</div>
                  {t.description && <div className="text-xs text-faint mt-0.5 line-clamp-1 max-w-xs">{t.description}</div>}
                </Cell>
                <Cell>
                  <span className={`font-mono text-[10px] px-2 py-0.5 rounded-full border ${t.target_type === 'agent' ? 'border-info/40 text-info' : 'border-accent/40 text-accent dark:text-accent-bright'}`}>{t.target_type}</span>
                  {t.target_name && <span className="text-xs text-muted-foreground ml-1.5">{t.target_name}</span>}
                </Cell>
                <Cell>
                  <span className="inline-flex items-center gap-1">
                    <code className="font-mono text-[11px] text-muted-foreground bg-muted px-2 py-0.5 rounded truncate max-w-[200px] inline-block align-middle">{url}</code>
                    <CopyButton text={url} />
                  </span>
                </Cell>
                <Cell>
                  <Link href={`/triggers/${t.id}?tab=runs`} className="font-mono text-[11px] text-muted-foreground hover:text-accent dark:hover:text-accent-bright">
                    {t.trigger_count}× {t.last_triggered_at ? `· ${formatDate(t.last_triggered_at)}` : ''}
                  </Link>
                </Cell>
                <Cell>
                  <button onClick={() => handleToggle(t)} title={t.is_active ? 'Deactivate' : 'Activate'}
                    className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${t.is_active ? 'bg-accent' : 'bg-muted'}`}>
                    <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-surface shadow transition-transform ${t.is_active ? 'translate-x-4' : 'translate-x-1'}`} />
                  </button>
                </Cell>
                <Cell className="text-right">
                  <span className="inline-flex items-center gap-0.5">
                    <Link href={`/triggers/${t.id}`} className="p-1.5 rounded text-faint hover:text-foreground hover:bg-muted" title="Edit"><Edit2 className="w-4 h-4" /></Link>
                    <button onClick={() => handleDelete(t.id)} className="p-1.5 rounded text-faint hover:text-crit hover:bg-crit/10" title="Delete"><Trash2 className="w-4 h-4" /></button>
                  </span>
                </Cell>
              </Row>
            )
          })}
        </DataTable>
      )}

      <div className="mt-6 p-4 rounded-lg bg-muted border border-border-strong">
        <p className="text-sm text-muted-foreground">
          Learn how to configure and secure webhook triggers in the{' '}
          <Link href="/docs/webhook-triggers" className="text-accent dark:text-accent-bright hover:underline">
            documentation
          </Link>.
        </p>
      </div>
    </div>
  )
}
