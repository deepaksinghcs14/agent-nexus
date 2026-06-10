'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { use } from 'react'
import { ChevronLeft, Check, Copy } from 'lucide-react'
import { webhookTriggersAPI } from '@/lib/api'
import type { WebhookTrigger } from '@/types'
import { TriggerForm } from '../TriggerForm'

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
      className="p-1.5 rounded hover:bg-gray-100 text-gray-400 hover:text-gray-600 transition-colors"
      title="Copy"
    >
      {copied ? <Check className="w-4 h-4 text-green-500" /> : <Copy className="w-4 h-4" />}
    </button>
  )
}

export default function EditTriggerPage({ params }: { params: Promise<{ triggerId: string }> }) {
  const { triggerId } = use(params)
  const [trigger, setTrigger] = useState<WebhookTrigger | null>(null)
  const [loading, setLoading] = useState(true)
  const [notFound, setNotFound] = useState(false)

  useEffect(() => {
    webhookTriggersAPI.get(triggerId)
      .then((t) => setTrigger(t as WebhookTrigger))
      .catch(() => setNotFound(true))
      .finally(() => setLoading(false))
  }, [triggerId])

  if (loading) {
    return <div className="py-20 text-center text-sm text-gray-400">Loading…</div>
  }

  if (notFound || !trigger) {
    return (
      <div className="max-w-2xl mx-auto py-20 px-6 text-center">
        <p className="text-gray-500 mb-4">Trigger not found.</p>
        <Link href="/triggers" className="text-[#534AB7] text-sm hover:underline">Back to triggers</Link>
      </div>
    )
  }

  const url = webhookURL(trigger.id)

  return (
    <div className="max-w-2xl mx-auto py-10 px-6">
      <Link
        href="/triggers"
        className="flex items-center gap-1 text-sm text-gray-500 hover:text-gray-700 mb-6 transition-colors"
      >
        <ChevronLeft className="w-4 h-4" />
        Webhook Triggers
      </Link>

      <h1 className="text-2xl font-bold text-gray-900 mb-1">Edit Trigger</h1>
      <p className="text-sm text-gray-500 mb-6">{trigger.name}</p>

      {/* Webhook URL display */}
      <div className="mb-8 p-4 rounded-lg bg-gray-50 border border-gray-200">
        <p className="text-xs font-medium text-gray-500 mb-2 uppercase tracking-wide">Webhook URL</p>
        <div className="flex items-center gap-2">
          <code className="flex-1 text-sm font-mono text-gray-700 truncate">{url}</code>
          <CopyButton text={url} />
        </div>
        <p className="mt-2 text-xs text-gray-400">
          POST to this URL to trigger a run. Optionally set a secret to enable HMAC-SHA256 signature verification.
        </p>
      </div>

      <TriggerForm trigger={trigger} />
    </div>
  )
}
