'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { agentsAPI, webhookTriggersAPI, workflowsAPI } from '@/lib/api'
import type { Agent, WebhookTrigger, Workflow } from '@/types'

interface Props {
  trigger?: WebhookTrigger
  prefillTargetType?: 'agent' | 'workflow'
  prefillTargetId?: string
}

const DEFAULT_TEMPLATE = '{{.RawBody}}'

export function TriggerForm({ trigger, prefillTargetType, prefillTargetId }: Props) {
  const router = useRouter()
  const isEdit = !!trigger

  const [name, setName] = useState(trigger?.name ?? '')
  const [description, setDescription] = useState(trigger?.description ?? '')
  const [targetType, setTargetType] = useState<'agent' | 'workflow'>(trigger?.target_type ?? prefillTargetType ?? 'agent')
  const [targetId, setTargetId] = useState(trigger?.target_id ?? prefillTargetId ?? '')
  const [inputTemplate, setInputTemplate] = useState(trigger?.input_template ?? DEFAULT_TEMPLATE)
  const [secret, setSecret] = useState(trigger?.secret ?? '')
  const [isActive, setIsActive] = useState(trigger?.is_active ?? true)

  const [agents, setAgents] = useState<Agent[]>([])
  const [workflows, setWorkflows] = useState<Workflow[]>([])

  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    agentsAPI.list().then((r) => setAgents(((r as { data?: Agent[] }).data) ?? [])).catch(() => {})
    workflowsAPI.list().then((r) => setWorkflows(((r as { data?: Workflow[] }).data) ?? [])).catch(() => {})
  }, [])

  // Reset target when target type changes (unless editing existing).
  const handleTargetTypeChange = (type: 'agent' | 'workflow') => {
    setTargetType(type)
    if (!isEdit) setTargetId('')
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (!name.trim()) { setError('Name is required'); return }
    if (!targetId) { setError('Please select a target'); return }

    setSaving(true)
    try {
      const payload = {
        name: name.trim(),
        description: description.trim(),
        target_type: targetType,
        target_id: targetId,
        input_template: inputTemplate || DEFAULT_TEMPLATE,
        secret: secret.trim(),
        is_active: isActive,
      }
      if (isEdit) {
        await webhookTriggersAPI.update(trigger.id, payload)
      } else {
        await webhookTriggersAPI.create(payload)
      }
      router.push('/triggers')
    } catch {
      setError('Failed to save trigger. Check that the target exists.')
    } finally {
      setSaving(false)
    }
  }

  const targets = targetType === 'agent' ? agents : workflows

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      {/* Name */}
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">
          Name <span className="text-red-500">*</span>
        </label>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. GitHub PR Reviewer"
          className="w-full px-3 py-2 rounded-md border border-gray-300 text-sm focus:outline-none focus:ring-2 focus:ring-[#534AB7]/30 focus:border-[#534AB7]"
        />
      </div>

      {/* Description */}
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
        <input
          type="text"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Optional — what does this trigger do?"
          className="w-full px-3 py-2 rounded-md border border-gray-300 text-sm focus:outline-none focus:ring-2 focus:ring-[#534AB7]/30 focus:border-[#534AB7]"
        />
      </div>

      {/* Target type */}
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-2">Target type</label>
        <div className="flex gap-3">
          {(['agent', 'workflow'] as const).map((type) => (
            <button
              key={type}
              type="button"
              onClick={() => handleTargetTypeChange(type)}
              className={`px-4 py-2 rounded-md border text-sm font-medium transition-colors capitalize ${
                targetType === type
                  ? 'bg-[#534AB7] border-[#534AB7] text-white'
                  : 'border-gray-300 text-gray-600 hover:border-gray-400'
              }`}
            >
              {type}
            </button>
          ))}
        </div>
      </div>

      {/* Target */}
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">
          Target {targetType} <span className="text-red-500">*</span>
        </label>
        <select
          value={targetId}
          onChange={(e) => setTargetId(e.target.value)}
          className="w-full px-3 py-2 rounded-md border border-gray-300 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-[#534AB7]/30 focus:border-[#534AB7]"
        >
          <option value="">Select a {targetType}…</option>
          {targets.map((t) => (
            <option key={t.id} value={t.id}>{t.name}</option>
          ))}
        </select>
      </div>

      {/* Input template */}
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Input template</label>
        <textarea
          value={inputTemplate}
          onChange={(e) => setInputTemplate(e.target.value)}
          rows={3}
          className="w-full px-3 py-2 rounded-md border border-gray-300 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-[#534AB7]/30 focus:border-[#534AB7]"
        />
        <p className="mt-1.5 text-xs text-gray-400 space-y-0.5">
          Go template evaluated against the inbound request. Available variables:
        </p>
        <div className="mt-1 grid grid-cols-2 gap-x-4 gap-y-0.5 text-xs text-gray-500">
          <span><code className="font-mono text-[#534AB7]">{'{{.RawBody}}'}</code> — full JSON body as string</span>
          <span><code className="font-mono text-[#534AB7]">{'{{.Body.field}}'}</code> — parsed JSON field</span>
          <span><code className="font-mono text-[#534AB7]">{'{{.Headers.X-Name}}'}</code> — request header</span>
          <span><code className="font-mono text-[#534AB7]">{'{{.Query.param}}'}</code> — query string param</span>
        </div>
      </div>

      {/* Secret */}
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">
          HMAC secret <span className="text-gray-400 font-normal">(optional)</span>
        </label>
        <input
          type="password"
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
          placeholder="Leave blank for no signature verification"
          className="w-full px-3 py-2 rounded-md border border-gray-300 text-sm focus:outline-none focus:ring-2 focus:ring-[#534AB7]/30 focus:border-[#534AB7]"
        />
        <p className="mt-1 text-xs text-gray-400">
          When set, inbound requests must include a valid{' '}
          <code className="font-mono">X-Hub-Signature-256: sha256=&lt;hex&gt;</code> header.
        </p>
      </div>

      {/* Active toggle */}
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={() => setIsActive(!isActive)}
          className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${isActive ? 'bg-[#534AB7]' : 'bg-gray-200'}`}
        >
          <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${isActive ? 'translate-x-4' : 'translate-x-1'}`} />
        </button>
        <span className="text-sm text-gray-700">{isActive ? 'Active' : 'Inactive'}</span>
      </div>

      {error && <p className="text-sm text-red-500">{error}</p>}

      <div className="flex gap-3 pt-2">
        <button
          type="submit"
          disabled={saving}
          className="px-5 py-2 rounded-md bg-[#534AB7] hover:bg-[#4a42a3] text-white text-sm font-medium transition-colors disabled:opacity-50"
        >
          {saving ? 'Saving…' : isEdit ? 'Save changes' : 'Create trigger'}
        </button>
        <button
          type="button"
          onClick={() => router.push('/triggers')}
          className="px-4 py-2 rounded-md border border-gray-300 text-sm text-gray-600 hover:bg-gray-50 transition-colors"
        >
          Cancel
        </button>
      </div>
    </form>
  )
}
