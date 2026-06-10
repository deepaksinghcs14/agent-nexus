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
    <button onClick={handle} className="p-1 rounded hover:bg-gray-100 text-gray-400 hover:text-gray-600 transition-colors" title="Copy URL">
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
    <div className="max-w-4xl mx-auto py-10 px-6">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Webhook Triggers</h1>
          <p className="text-sm text-gray-500 mt-1">
            Inbound HTTP events that automatically run an agent or workflow.
          </p>
        </div>
        <Link
          href="/triggers/new"
          className="flex items-center gap-2 px-4 py-2 rounded-md bg-[#534AB7] hover:bg-[#4a42a3] text-white text-sm font-medium transition-colors"
        >
          <Plus className="w-4 h-4" />
          New trigger
        </Link>
      </div>

      {loading ? (
        <div className="py-10 text-center text-sm text-gray-400">Loading…</div>
      ) : triggers.length === 0 ? (
        <div className="py-16 text-center">
          <Zap className="w-10 h-10 text-gray-300 mx-auto mb-3" />
          <p className="text-sm font-medium text-gray-600 mb-1">No webhook triggers yet</p>
          <p className="text-sm text-gray-400 mb-4">
            Create a trigger to run agents or workflows from external HTTP events.
          </p>
          <Link
            href="/triggers/new"
            className="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-[#534AB7] text-white text-sm font-medium hover:bg-[#4a42a3] transition-colors"
          >
            <Plus className="w-4 h-4" />
            Create your first trigger
          </Link>
        </div>
      ) : (
        <div className="rounded-lg border border-gray-200 overflow-hidden divide-y divide-gray-100">
          {triggers.map((t) => {
            const url = webhookURL(t.id)
            return (
              <div key={t.id} className="bg-white px-5 py-4">
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2 mb-0.5">
                      <span className="text-sm font-medium text-gray-900">{t.name}</span>
                      <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${t.target_type === 'agent' ? 'bg-blue-50 text-blue-600' : 'bg-purple-50 text-purple-600'}`}>
                        {t.target_type}
                      </span>
                      {t.target_name && (
                        <span className="text-xs text-gray-500">{t.target_name}</span>
                      )}
                    </div>
                    {t.description && (
                      <p className="text-xs text-gray-400 mb-1">{t.description}</p>
                    )}
                    <div className="flex items-center gap-1 mt-1.5">
                      <code className="text-xs text-gray-500 font-mono bg-gray-50 px-2 py-0.5 rounded truncate max-w-xs">
                        {url}
                      </code>
                      <CopyButton text={url} />
                    </div>
                    <div className="flex items-center gap-3 mt-1.5 text-xs text-gray-400">
                      <span>{t.trigger_count} {t.trigger_count === 1 ? 'trigger' : 'triggers'}</span>
                      {t.last_triggered_at && <span>Last fired {formatDate(t.last_triggered_at)}</span>}
                      <span>Created {formatDate(t.created_at)}</span>
                    </div>
                  </div>

                  <div className="flex items-center gap-2 shrink-0">
                    <button
                      onClick={() => handleToggle(t)}
                      title={t.is_active ? 'Deactivate' : 'Activate'}
                      className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${t.is_active ? 'bg-[#534AB7]' : 'bg-gray-200'}`}
                    >
                      <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${t.is_active ? 'translate-x-4' : 'translate-x-1'}`} />
                    </button>
                    <Link
                      href={`/triggers/${t.id}`}
                      className="p-1.5 rounded text-gray-400 hover:text-gray-600 hover:bg-gray-100 transition-colors"
                      title="Edit"
                    >
                      <Edit2 className="w-4 h-4" />
                    </Link>
                    <button
                      onClick={() => handleDelete(t.id)}
                      className="p-1.5 rounded text-gray-400 hover:text-red-500 hover:bg-red-50 transition-colors"
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

      <div className="mt-6 p-4 rounded-lg bg-gray-50 border border-gray-200">
        <p className="text-sm text-gray-600">
          Learn how to configure and secure webhook triggers in the{' '}
          <Link href="/docs/webhook-triggers" className="text-[#534AB7] hover:underline">
            documentation
          </Link>.
        </p>
      </div>
    </div>
  )
}
