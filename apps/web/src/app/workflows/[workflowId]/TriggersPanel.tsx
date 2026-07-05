'use client'

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useRouter } from 'next/navigation'
import { Check, Copy, Plus, Trash2, X, Zap } from 'lucide-react'
import { webhookTriggersAPI } from '@/lib/api'
import type { WebhookTrigger } from '@/types'
import { cn } from '@/lib/utils'
import { Switch } from '@/components/ui/switch'

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'
function webhookURL(id: string) { return `${API_URL}/webhook/${id}` }

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  const handle = () => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }
  return (
    <button
      onClick={handle}
      title="Copy URL"
      className={cn('bg-transparent border-none cursor-pointer p-0.5', copied ? 'text-green-500' : 'text-gray-400')}
    >
      {copied ? <Check size={12} /> : <Copy size={12} />}
    </button>
  )
}

interface Props {
  workflowId: string
  onClose: () => void
}

export function TriggersPanel({ workflowId, onClose }: Props) {
  const router = useRouter()
  const qc = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['webhook-triggers'],
    queryFn: () => webhookTriggersAPI.list() as Promise<{ data: WebhookTrigger[] }>,
  })

  const triggers = (data?.data ?? []).filter(
    t => t.target_type === 'workflow' && t.target_id === workflowId,
  )

  const toggleActive = useMutation({
    mutationFn: (t: WebhookTrigger) => webhookTriggersAPI.update(t.id, { is_active: !t.is_active }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['webhook-triggers'] }),
  })

  const remove = useMutation({
    mutationFn: (id: string) => webhookTriggersAPI.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['webhook-triggers'] }),
  })

  const handleDelete = (t: WebhookTrigger) => {
    if (!confirm(`Delete trigger "${t.name}"? External services using this URL will stop working.`)) return
    remove.mutate(t.id)
  }

  return (
    <div className="w-full sm:w-72 flex-shrink-0 bg-white border-l border-gray-200 flex flex-col overflow-y-auto">
      {/* Header */}
      <div className="px-3.5 py-2.5 border-b border-gray-100 flex items-center justify-between flex-shrink-0">
        <div className="flex items-center gap-1.5">
          <Zap size={13} className="text-accent" />
          <span className="text-xs font-bold text-gray-700 uppercase tracking-wide">Webhook Triggers</span>
        </div>
        <button onClick={onClose} className="bg-transparent border-none cursor-pointer text-gray-400">
          <X size={14} />
        </button>
      </div>

      {/* Body */}
      <div className="flex-1 p-3.5 flex flex-col gap-2">
        {isLoading && (
          <p className="text-xs text-gray-400 text-center pt-6">Loading…</p>
        )}

        {!isLoading && triggers.length === 0 && (
          <div className="text-center pt-8">
            <div className="w-9 h-9 rounded-full bg-accent-light flex items-center justify-center mx-auto mb-2.5">
              <Zap size={16} className="text-accent" />
            </div>
            <p className="text-xs font-semibold text-gray-700 mb-1">No triggers yet</p>
            <p className="text-[11px] text-gray-400 mb-4 leading-relaxed">
              Add a webhook to fire this workflow from GitHub, Zapier, or any HTTP source.
            </p>
          </div>
        )}

        {triggers.map(t => {
          const url = webhookURL(t.id)
          return (
            <div key={t.id} className="border border-gray-200 rounded-lg px-3 py-2.5 bg-gray-50">
              <div className="flex items-start justify-between gap-1.5 mb-1.5">
                <span className="text-xs font-semibold text-gray-900 flex-1 break-words">{t.name}</span>
                <button
                  onClick={() => handleDelete(t)}
                  className="bg-transparent border-none cursor-pointer text-gray-300 p-0.5 flex-shrink-0"
                  title="Delete trigger"
                >
                  <Trash2 size={12} />
                </button>
              </div>

              {/* Active toggle */}
              <div className="flex items-center gap-1.5 mb-2">
                <Switch
                  checked={t.is_active}
                  onCheckedChange={() => toggleActive.mutate(t)}
                  className="data-[state=checked]:bg-accent"
                />
                <span className={cn('text-[10px] font-semibold', t.is_active ? 'text-accent' : 'text-gray-400')}>
                  {t.is_active ? 'Active' : 'Inactive'}
                </span>
                {t.trigger_count > 0 && (
                  <span className="text-[10px] text-gray-400 ml-auto">
                    {t.trigger_count} {t.trigger_count === 1 ? 'fire' : 'fires'}
                  </span>
                )}
              </div>

              {/* URL */}
              <div className="flex items-center gap-1 bg-gray-100 rounded-md px-2 py-1">
                <code className="text-[9px] text-gray-500 flex-1 overflow-hidden text-ellipsis whitespace-nowrap">
                  {url}
                </code>
                <CopyButton text={url} />
              </div>
            </div>
          )
        })}
      </div>

      {/* Footer: add trigger */}
      <div className="px-3.5 py-2.5 border-t border-gray-100 flex-shrink-0">
        <button
          onClick={() => router.push(`/triggers/new?target_type=workflow&target_id=${workflowId}`)}
          className="w-full flex items-center justify-center gap-1.5 py-1.5 px-3 rounded-lg text-xs font-semibold cursor-pointer bg-accent text-white border-none hover:bg-accent-hover transition-colors"
        >
          <Plus size={12} /> Add trigger
        </button>
      </div>
    </div>
  )
}
