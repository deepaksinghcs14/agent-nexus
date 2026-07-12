'use client'

import { use, useEffect, useState, Suspense } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { ChevronLeft } from 'lucide-react'
import { webhookTriggersAPI, webhookURL } from '@/lib/api'
import type { WebhookTrigger } from '@/types'
import { TriggerForm } from '../TriggerForm'
import { TriggerRunsTab } from './TriggerRunsTab'
import { CopyButton } from '@/components/ui/CopyButton'

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
    return <div className="py-20 text-center text-sm text-faint">Loading…</div>
  }

  if (notFound || !trigger) {
    return (
      <div className="p-6 text-center">
        <p className="text-muted-foreground mb-4">Trigger not found.</p>
        <Link href="/triggers" className="text-accent dark:text-accent-bright text-sm hover:underline">Back to triggers</Link>
      </div>
    )
  }

  const url = webhookURL(trigger.id)

  return (
    <div className="p-4 sm:p-6">
      <Link
        href="/triggers"
        className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground dark:hover:text-faint mb-6 transition-colors"
      >
        <ChevronLeft className="w-4 h-4" />
        Webhook Triggers
      </Link>

      <h1 className="text-xl font-semibold text-foreground mb-0.5">{trigger.name}</h1>
      {trigger.description && (
        <p className="text-sm text-muted-foreground mb-6">{trigger.description}</p>
      )}

      {/* Webhook URL — always visible */}
      <div className="mb-6 p-4 rounded-lg bg-muted border border-border-strong">
        <p className="text-xs font-medium text-muted-foreground mb-2 uppercase tracking-wide">Webhook URL</p>
        <div className="flex items-center gap-2">
          <code className="flex-1 text-sm font-mono text-foreground truncate">{url}</code>
          <CopyButton text={url} />
        </div>
        <p className="mt-2 text-xs text-faint">
          POST to this URL to trigger a run. Optionally set a secret to enable HMAC-SHA256 signature verification.
        </p>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 mb-6 border-b border-border-strong">
        {(['config', 'runs'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`px-4 py-2 text-sm font-medium capitalize border-b-2 transition-colors -mb-px ${
              tab === t
                ? 'border-accent/50 text-accent dark:text-accent-bright'
                : 'border-transparent text-muted-foreground hover:text-foreground dark:hover:text-faint'
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
    <Suspense fallback={<div className="py-20 text-center text-sm text-faint">Loading…</div>}>
      <TriggerPageInner triggerId={triggerId} />
    </Suspense>
  )
}
