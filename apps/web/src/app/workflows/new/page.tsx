'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { useMutation } from '@tanstack/react-query'
import { workflowsAPI } from '@/lib/api'
import type { Workflow } from '@/types'

export default function NewWorkflowPage() {
  const router = useRouter()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [mode, setMode] = useState<'pipeline' | 'supervisor'>('pipeline')
  const [error, setError] = useState('')

  const create = useMutation({
    mutationFn: () => workflowsAPI.create({ name, description, mode }) as Promise<Workflow>,
    onSuccess: (wf: Workflow) => router.push(`/workflows/${wf.id}`),
    onError: (err: Error) => setError(err.message),
  })

  return (
    <div className="p-4 sm:p-6 max-w-2xl">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-gray-900">Create Workflow</h1>
        <p className="text-sm text-gray-500 mt-0.5">
          Set up a name and mode, then build your workflow visually on the canvas.
        </p>
      </div>
      <div className="bg-white border border-gray-100 rounded-xl p-6 space-y-4">
        {error && <div className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg p-2">{error}</div>}
        <Field label="Workflow name">
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg"
            placeholder="e.g. Research pipeline"
          />
        </Field>
        <Field label="Description">
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg resize-none"
            rows={2}
            placeholder="What does this workflow do?"
          />
        </Field>
        <Field label="Mode">
          <select
            value={mode}
            onChange={(e) => setMode(e.target.value as typeof mode)}
            className="w-full px-3 py-2 text-sm border border-gray-200 rounded-lg bg-white"
          >
            <option value="pipeline">Pipeline — nodes execute sequentially</option>
            <option value="supervisor">Supervisor — one agent coordinates others</option>
          </select>
        </Field>
        <div className="pt-2 flex gap-2 justify-end">
          <Link href="/workflows" className="px-4 py-2 text-sm text-gray-600 border border-gray-200 rounded-lg">
            Cancel
          </Link>
          <button
            onClick={() => create.mutate()}
            disabled={!name.trim() || create.isPending}
            className="px-4 py-2 bg-purple-600 text-white text-sm font-medium rounded-lg disabled:opacity-50"
          >
            {create.isPending ? 'Creating…' : 'Create & open canvas →'}
          </button>
        </div>
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-xs font-medium text-gray-700 mb-1">{label}</label>
      {children}
    </div>
  )
}
