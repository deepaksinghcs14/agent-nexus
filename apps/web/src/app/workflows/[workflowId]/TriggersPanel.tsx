'use client'

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useRouter } from 'next/navigation'
import { Check, Copy, Plus, Trash2, X, Zap } from 'lucide-react'
import { webhookTriggersAPI } from '@/lib/api'
import type { WebhookTrigger } from '@/types'

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
    <button onClick={handle} title="Copy URL" style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '2px', color: copied ? '#22c55e' : '#9ca3af' }}>
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
    <div style={{
      width: 280, flexShrink: 0, background: '#fff', borderLeft: '1px solid #e5e7eb',
      display: 'flex', flexDirection: 'column', overflowY: 'auto',
    }}>
      {/* Header */}
      <div style={{ padding: '12px 14px 10px', borderBottom: '1px solid #f3f4f6', display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexShrink: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <Zap size={13} color="#7c3aed" />
          <span style={{ fontSize: 12, fontWeight: 700, color: '#374151', textTransform: 'uppercase', letterSpacing: 0.5 }}>
            Webhook Triggers
          </span>
        </div>
        <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#9ca3af' }}>
          <X size={14} />
        </button>
      </div>

      {/* Body */}
      <div style={{ flex: 1, padding: '12px 14px', display: 'flex', flexDirection: 'column', gap: 8 }}>
        {isLoading && (
          <p style={{ fontSize: 12, color: '#9ca3af', textAlign: 'center', paddingTop: 24 }}>Loading…</p>
        )}

        {!isLoading && triggers.length === 0 && (
          <div style={{ textAlign: 'center', paddingTop: 32 }}>
            <div style={{ width: 36, height: 36, borderRadius: '50%', background: '#f1f0ff', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 10px' }}>
              <Zap size={16} color="#7c3aed" />
            </div>
            <p style={{ fontSize: 12, fontWeight: 600, color: '#374151', marginBottom: 4 }}>No triggers yet</p>
            <p style={{ fontSize: 11, color: '#9ca3af', marginBottom: 16, lineHeight: 1.5 }}>
              Add a webhook to fire this workflow from GitHub, Zapier, or any HTTP source.
            </p>
          </div>
        )}

        {triggers.map(t => {
          const url = webhookURL(t.id)
          return (
            <div key={t.id} style={{ border: '1px solid #e5e7eb', borderRadius: 8, padding: '10px 12px', background: '#fafafa' }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 6, marginBottom: 6 }}>
                <span style={{ fontSize: 12, fontWeight: 600, color: '#111827', flex: 1, wordBreak: 'break-word' }}>{t.name}</span>
                <button
                  onClick={() => handleDelete(t)}
                  style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#d1d5db', padding: 2, flexShrink: 0 }}
                  title="Delete trigger"
                >
                  <Trash2 size={12} />
                </button>
              </div>

              {/* Active toggle */}
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8 }}>
                <button
                  onClick={() => toggleActive.mutate(t)}
                  style={{
                    width: 28, height: 16, borderRadius: 8, border: 'none', cursor: 'pointer',
                    background: t.is_active ? '#7c3aed' : '#d1d5db', position: 'relative', flexShrink: 0,
                  }}
                >
                  <span style={{
                    position: 'absolute', top: 2, left: t.is_active ? 14 : 2,
                    width: 12, height: 12, borderRadius: '50%', background: '#fff',
                    transition: 'left 0.15s',
                  }} />
                </button>
                <span style={{ fontSize: 10, color: t.is_active ? '#7c3aed' : '#9ca3af', fontWeight: 600 }}>
                  {t.is_active ? 'Active' : 'Inactive'}
                </span>
                {t.trigger_count > 0 && (
                  <span style={{ fontSize: 10, color: '#9ca3af', marginLeft: 'auto' }}>
                    {t.trigger_count} {t.trigger_count === 1 ? 'fire' : 'fires'}
                  </span>
                )}
              </div>

              {/* URL */}
              <div style={{ display: 'flex', alignItems: 'center', gap: 4, background: '#f3f4f6', borderRadius: 6, padding: '4px 8px' }}>
                <code style={{ fontSize: 9, color: '#6b7280', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {url}
                </code>
                <CopyButton text={url} />
              </div>
            </div>
          )
        })}
      </div>

      {/* Footer: add trigger */}
      <div style={{ padding: '10px 14px', borderTop: '1px solid #f3f4f6', flexShrink: 0 }}>
        <button
          onClick={() => router.push(`/triggers/new?target_type=workflow&target_id=${workflowId}`)}
          style={{
            width: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
            padding: '7px 12px', borderRadius: 8, fontSize: 12, fontWeight: 600, cursor: 'pointer',
            background: '#7c3aed', color: '#fff', border: 'none',
          }}
        >
          <Plus size={12} /> Add trigger
        </button>
      </div>
    </div>
  )
}
