'use client'

import { use, useEffect, useState } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { Suspense } from 'react'
import { ChevronLeft, Check, Copy } from 'lucide-react'
import { webhookTriggersAPI } from '@/lib/api'
import type { WebhookTrigger } from '@/types'
import { TriggerForm } from '../TriggerForm'
import { TriggerRunsTab } from './TriggerRunsTab'

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'

function webhookURL(id: string) {
  return `${API_URL}/webhook/${id}`
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  const handle = () => {
    navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }
  return (
    <button
      onClick={handle}
      className="p-1.5 rounded hover:bg-gray-100 dark:hover:bg-gray-800 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
      title="Copy"
    >
      {copied ? <Check className="w-4 h-4 text-green-500" /> : <Copy className="w-4 h-4" />}
    </button>
  )
}

type Tab = 'config' | 'runs'

function TriggerPageInner({ triggerId }: { triggerId: string }) {
  const searchParams = useSearchParams()
  const [trigger, setTrigger] = useState<WebhookTrigger | null>(null)
  const [loading, setLoading] = useState(true)
  const [notFound, setNotFound] = useState(false)
  const [tab, setTab] = useState<Tab>((searchParams.get('tab') as Tab) ?? 'config')

  useEffect(() => {
    webhookTriggersAPI.get(triggerId)
      .then((t) => setTrigger(t as WebhookTrigger))
      .catch(() => setNotFound(true))
      .finally(() => setLoading(false))
  }, [triggerId])

  if (loading) {
    return <div className="py-20 text-center text-sm text-gray-400 dark:text-gray-500">Loading…</div>
  }

  if (notFound || !trigger) {
    return (
      <div className="p-6 text-center">
        <p className="text-gray-500 dark:text-gray-400 mb-4">Trigger not found.</p>
        <Link href="/triggers" className="text-purple-600 dark:text-purple-300 text-sm hover:underline">Back to triggers</Link>
      </div>
    )
  }

  const url = webhookURL(trigger.id)

  return (
    <div className="p-4 sm:p-6">
      <Link
        href="/triggers"
        className="flex items-center gap-1 text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300 mb-6 transition-colors"
      >
        <ChevronLeft className="w-4 h-4" />
        Webhook Triggers
      </Link>

      <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100 mb-0.5">{trigger.name}</h1>
      {trigger.description && (
        <p className="text-sm text-gray-500 dark:text-gray-400 mb-6">{trigger.description}</p>
      )}

      {/* Webhook URL — always visible */}
      <div className="mb-6 p-4 rounded-lg bg-gray-50 dark:bg-gray-800/60 border border-gray-200 dark:border-gray-700">
        <p className="text-xs font-medium text-gray-500 dark:text-gray-400 mb-2 uppercase tracking-wide">Webhook URL</p>
        <div className="flex items-center gap-2">
          <code className="flex-1 text-sm font-mono text-gray-700 dark:text-gray-300 truncate">{url}</code>
          <CopyButton text={url} />
        </div>
        <p className="mt-2 text-xs text-gray-400 dark:text-gray-500">
          POST to this URL to trigger a run. Optionally set a secret to enable HMAC-SHA256 signature verification.
        </p>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 mb-6 border-b border-gray-200 dark:border-gray-700">
        {(['config', 'runs'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`px-4 py-2 text-sm font-medium capitalize border-b-2 transition-colors -mb-px ${
              tab === t
                ? 'border-purple-400 text-purple-600 dark:text-purple-300'
                : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'
            }`}
          >
            {t === 'config' ? 'Configuration' : 'Runs'}
          </button>
        ))}
      </div>

      {tab === 'config' && <TriggerForm trigger={trigger} />}
      {tab === 'runs' && <TriggerRunsTab triggerId={triggerId} />}
    </div>
  )
}

export default function EditTriggerPage({ params }: { params: Promise<{ triggerId: string }> }) {
  const { triggerId } = use(params)
  return (
    <Suspense fallback={<div className="py-20 text-center text-sm text-gray-400 dark:text-gray-500">Loading…</div>}>
      <TriggerPageInner triggerId={triggerId} />
    </Suspense>
  )
}
