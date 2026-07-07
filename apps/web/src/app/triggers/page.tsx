'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Check, Copy, Edit2, Plus, Trash2, Zap } from 'lucide-react'
import { webhookTriggersAPI } from '@/lib/api'
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
    <button onClick={handle} className="p-1 rounded hover:bg-muted text-faint hover:text-gray-600 dark:hover:text-gray-300 transition-colors" title="Copy URL">
      {copied ? <Check className="w-3.5 h-3.5 text-green-500" /> : <Copy className="w-3.5 h-3.5" />}
    </button>
  )
}

export default function TriggersPage() {
  const [triggers, setTriggers] = useState<WebhookTrigger[]>([])
  const [loading, setLoading] = useState(true)

  const load = () => {
    webhookTriggersAPI.list()
      .then((r) => setTriggers(((r as { data?: WebhookTrigger[] }).data) ?? []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this webhook trigger? Any external services using this URL will stop working.')) return
    await webhookTriggersAPI.delete(id).catch(() => {})
    load()
  }

  const handleToggle = async (t: WebhookTrigger) => {
    await webhookTriggersAPI.update(t.id, { is_active: !t.is_active }).catch(() => {})
    load()
  }

  return (
    <div className="p-4 sm:p-6">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-8">
        <div>
          <span className="eyebrow block mb-1">Integrations</span>
          <h1 className="text-[22px] font-bold tracking-tight text-foreground">Triggers</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Inbound HTTP events that automatically run an agent or workflow.
          </p>
        </div>
        <Link
          href="/triggers/new"
          className="flex items-center gap-2 px-4 py-2 rounded-md bg-accent hover:bg-accent-hover text-white text-sm font-medium transition-colors"
        >
          <Plus className="w-4 h-4" />
          New trigger
        </Link>
      </div>

      {loading ? (
        <div className="py-10 text-center text-sm text-faint">Loading…</div>
      ) : triggers.length === 0 ? (
        <div className="py-16 text-center">
          <Zap className="w-10 h-10 text-faint mx-auto mb-3" />
          <p className="text-sm font-medium text-muted-foreground mb-1">No webhook triggers yet</p>
          <p className="text-sm text-faint mb-4">
            Create a trigger to run agents or workflows from external HTTP events.
          </p>
          <Link
            href="/triggers/new"
            className="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-accent text-white text-sm font-medium hover:bg-accent-hover transition-colors"
          >
            <Plus className="w-4 h-4" />
            Create your first trigger
          </Link>
        </div>
      ) : (
        <div className="rounded-lg border border-border-strong overflow-hidden divide-y divide-border">
          {triggers.map((t) => {
            const url = webhookURL(t.id)
            return (
              <div key={t.id} className="bg-surface px-4 sm:px-5 py-4">
                <div className="flex flex-wrap items-start justify-between gap-4">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2 mb-0.5">
                      <span className="text-sm font-medium text-foreground">{t.name}</span>
                      <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${t.target_type === 'agent' ? 'bg-blue-50 dark:bg-blue-500/10 text-blue-600 dark:text-blue-300' : 'bg-accent/10 text-accent dark:text-accent-bright'}`}>
                        {t.target_type}
                      </span>
                      {t.target_name && (
                        <span className="text-xs text-muted-foreground">{t.target_name}</span>
                      )}
                    </div>
                    {t.description && (
                      <p className="text-xs text-faint mb-1">{t.description}</p>
                    )}
                    <div className="flex items-center gap-1 mt-1.5">
                      <code className="text-xs text-muted-foreground font-mono bg-muted px-2 py-0.5 rounded truncate max-w-[180px] sm:max-w-xs">
                        {url}
                      </code>
                      <CopyButton text={url} />
                    </div>
                    <div className="flex flex-wrap items-center gap-3 mt-1.5 text-xs text-faint">
                      <Link
                        href={`/triggers/${t.id}?tab=runs`}
                        className="hover:text-accent dark:text-accent-bright transition-colors"
                      >
                        {t.trigger_count} {t.trigger_count === 1 ? 'invocation' : 'invocations'}
                      </Link>
                      {t.last_triggered_at && <span>Last fired {formatDate(t.last_triggered_at)}</span>}
                      <span>Created {formatDate(t.created_at)}</span>
                    </div>
                  </div>

                  <div className="flex items-center gap-2 shrink-0">
                    <button
                      onClick={() => handleToggle(t)}
                      title={t.is_active ? 'Deactivate' : 'Activate'}
                      className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${t.is_active ? 'bg-accent' : 'bg-muted'}`}
                    >
                      <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-surface shadow transition-transform ${t.is_active ? 'translate-x-4' : 'translate-x-1'}`} />
                    </button>
                    <Link
                      href={`/triggers/${t.id}`}
                      className="p-1.5 rounded text-faint hover:text-gray-600 dark:hover:text-gray-300 hover:bg-muted transition-colors"
                      title="Edit"
                    >
                      <Edit2 className="w-4 h-4" />
                    </Link>
                    <button
                      onClick={() => handleDelete(t.id)}
                      className="p-1.5 rounded text-faint hover:text-red-500 hover:bg-red-50 transition-colors"
                      title="Delete"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>
              </div>
            )
          })}
        </div>
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
