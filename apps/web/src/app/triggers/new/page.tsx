'use client'

import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { Suspense } from 'react'
import { ChevronLeft } from 'lucide-react'
import { TriggerForm } from '../TriggerForm'

function NewTriggerForm() {
  const params = useSearchParams()
  const targetType = (params.get('target_type') ?? undefined) as 'agent' | 'workflow' | undefined
  const targetId = params.get('target_id') ?? undefined
  return <TriggerForm prefillTargetType={targetType} prefillTargetId={targetId} />
}

export default function NewTriggerPage() {
  return (
    <div className="p-4 sm:p-6">
      <Link
        href="/triggers"
        className="flex items-center gap-1 text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300 mb-6 transition-colors"
      >
        <ChevronLeft className="w-4 h-4" />
        Webhook Triggers
      </Link>

      <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100 mb-0.5">New Webhook Trigger</h1>
      <p className="text-sm text-gray-500 dark:text-gray-400 mb-8">
        Create an inbound HTTP endpoint that fires an agent or workflow run.
      </p>

      <Suspense fallback={null}>
        <NewTriggerForm />
      </Suspense>
    </div>
  )
}
