'use client'

import { useRef, useMemo, useState } from 'react'
import Link from 'next/link'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, MessageSquare, Pencil, Trash2, Bot, Download, Upload } from 'lucide-react'
import { agentsAPI } from '@/lib/api'
import { Skeleton } from '@/components/ui/skeleton'
import type { Agent } from '@/types'

const statusColors: Record<string, string> = {
  active: 'bg-green-50 dark:bg-green-500/10 text-green-700 dark:text-green-300',
  paused: 'bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300',
  archived: 'bg-gray-50 dark:bg-gray-800/60 text-gray-500 dark:text-gray-400',
}

const providerColors: Record<string, string> = {
  anthropic: 'bg-amber-50 dark:bg-amber-500/10 text-amber-800 dark:text-amber-300',
  openai: 'bg-green-50 dark:bg-green-500/10 text-green-800 dark:text-green-300',
  gemini: 'bg-blue-50 dark:bg-blue-500/10 text-blue-800 dark:text-blue-300',
  ollama: 'bg-gray-50 dark:bg-gray-800/60 text-gray-700 dark:text-gray-300',
}

export default function AgentsPage() {
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [importError, setImportError] = useState('')
  const fileInputRef = useRef<HTMLInputElement>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['agents'],
    queryFn: () => agentsAPI.list() as Promise<{ data: Agent[] }>,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => agentsAPI.delete(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['agents'] }),
  })

  const bulkDeleteMutation = useMutation({
    mutationFn: async (ids: string[]) => {
      await Promise.all(ids.map((id) => agentsAPI.delete(id)))
    },
    onSuccess: () => {
      setSelected(new Set())
      queryClient.invalidateQueries({ queryKey: ['agents'] })
    },
  })

  const importMutation = useMutation({
    mutationFn: (body: unknown) => agentsAPI.importAgent(body),
    onSuccess: () => {
      setImportError('')
      queryClient.invalidateQueries({ queryKey: ['agents'] })
    },
    onError: (err: Error) => setImportError(err.message),
  })

  function handleImportFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => {
      try {
        const parsed = JSON.parse(ev.target?.result as string)
        importMutation.mutate(parsed)
      } catch {
        setImportError('Invalid JSON file')
      }
    }
    reader.readAsText(file)
    e.target.value = ''
  }

  function exportAgent(agent: Agent) {
    const token = typeof window !== 'undefined' ? localStorage.getItem('access_token') : null
    const url = agentsAPI.exportUrl(agent.id)
    fetch(url, { headers: token ? { Authorization: `Bearer ${token}` } : {} })
      .then(r => r.blob())
      .then(blob => {
        const a = document.createElement('a')
        a.href = URL.createObjectURL(blob)
        a.download = `agent_${agent.name.toLowerCase().replace(/\s+/g, '_')}.json`
        a.click()
        URL.revokeObjectURL(a.href)
      })
  }

  const agents = useMemo(() => data?.data ?? [], [data?.data])
  const visibleIds = useMemo(() => agents.filter((a) => !a.protected).map((a) => a.id), [agents])
  const selectedCount = selected.size
  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selected.has(id))

  function toggleSelected(id: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function toggleAllVisible() {
    setSelected((prev) => {
      const next = new Set(prev)
      if (allVisibleSelected) {
        visibleIds.forEach((id) => next.delete(id))
      } else {
        visibleIds.forEach((id) => next.add(id))
      }
      return next
    })
  }

  function removeSelected() {
    const ids = Array.from(selected).filter((id) => visibleIds.includes(id))
    if (ids.length === 0) return
    if (confirm(`Delete ${ids.length} selected agent${ids.length !== 1 ? 's' : ''}?`)) {
      bulkDeleteMutation.mutate(ids)
    }
  }

  return (
    <div className="p-4 sm:p-6 max-w-6xl">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-6">
        <div>
          <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">Agents</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">{agents.length} agent{agents.length !== 1 ? 's' : ''}</p>
        </div>
        <div className="flex items-center gap-2">
          <input ref={fileInputRef} type="file" accept=".json" className="hidden" onChange={handleImportFile} />
          <button
            onClick={() => { setImportError(''); fileInputRef.current?.click() }}
            disabled={importMutation.isPending}
            className="flex items-center gap-1.5 px-3 py-1.5 border border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400 text-sm rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800 disabled:opacity-60"
          >
            <Upload size={15} /> {importMutation.isPending ? 'Importing…' : 'Import'}
          </button>
          <Link href="/agents/new">
            <button className="flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-sm rounded-lg hover:bg-purple-700">
              <Plus size={15} /> New Agent
            </button>
          </Link>
        </div>
      </div>
      {importError && (
        <div className="mb-4 text-sm text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10 border border-red-200 rounded-lg px-3 py-2">{importError}</div>
      )}

      {!isLoading && agents.length > 0 && (
        <div className="flex items-center justify-between mb-3 rounded-lg border border-gray-100 dark:border-gray-800 bg-white dark:bg-gray-900 px-3 py-2">
          <label className="flex items-center gap-2 text-[12px] text-gray-600 dark:text-gray-400">
            <input
              type="checkbox"
              checked={allVisibleSelected}
              onChange={toggleAllVisible}
              className="h-4 w-4 rounded border-gray-300 dark:border-gray-600 text-purple-600 dark:text-purple-300 focus:ring-purple-500"
            />
            Select all visible
          </label>
          <div className="flex items-center gap-2">
            <span className="text-[11px] text-gray-400 dark:text-gray-500">{selectedCount} selected</span>
            <button
              onClick={removeSelected}
              disabled={selectedCount === 0 || bulkDeleteMutation.isPending}
              className="inline-flex items-center gap-1.5 rounded-md border border-red-200 px-2.5 py-1 text-[11px] font-medium text-red-600 dark:text-red-300 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-40"
            >
              <Trash2 size={13} />
              {bulkDeleteMutation.isPending ? 'Removing...' : 'Remove selected'}
            </button>
          </div>
        </div>
      )}

      {isLoading && (
        <div className="grid gap-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="border border-gray-100 dark:border-gray-800 rounded-xl p-4 bg-white dark:bg-gray-900 flex items-start gap-3">
              <Skeleton className="h-5 w-5 rounded flex-shrink-0" />
              <div className="flex-1 min-w-0">
                <Skeleton className="h-4 w-40 mb-2" />
                <Skeleton className="h-3 w-64 mb-2" />
                <Skeleton className="h-3 w-24" />
              </div>
            </div>
          ))}
        </div>
      )}

      {!isLoading && agents.length === 0 && (
        <div className="border border-dashed border-gray-200 dark:border-gray-700 rounded-xl p-12 text-center">
          <Bot size={32} className="mx-auto text-gray-300 dark:text-gray-600 mb-3" />
          <p className="text-gray-500 dark:text-gray-400 text-sm">No agents yet.</p>
          <Link href="/agents/new">
            <button className="mt-4 px-4 py-2 bg-purple-600 text-white text-sm rounded-lg hover:bg-purple-700">
              Create your first agent
            </button>
          </Link>
        </div>
      )}

      <div className="grid gap-3">
        {agents.map((agent) => (
          <div key={agent.id} className={`border rounded-xl p-4 bg-white dark:bg-gray-900 hover:border-purple-200 hover:shadow-sm transition-all ${selected.has(agent.id) ? 'border-purple-200 ring-1 ring-purple-100' : 'border-gray-100 dark:border-gray-800'}`}>
            <div className="flex items-start justify-between">
              <label className="mr-3 mt-0.5 flex h-5 w-5 items-center justify-center">
                {!agent.protected && (
                  <input
                    type="checkbox"
                    checked={selected.has(agent.id)}
                    onChange={() => toggleSelected(agent.id)}
                    className="h-4 w-4 rounded border-gray-300 dark:border-gray-600 text-purple-600 dark:text-purple-300 focus:ring-purple-500"
                    aria-label={`Select ${agent.name}`}
                  />
                )}
              </label>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <span className="font-medium text-gray-900 dark:text-gray-100 text-sm">{agent.name}</span>
                  <span className={`text-[11px] px-2 py-0.5 rounded-full font-medium ${statusColors[agent.status] ?? statusColors.active}`}>
                    {agent.status}
                  </span>
                  <span className={`text-[11px] px-2 py-0.5 rounded-full font-medium ${providerColors[agent.provider] ?? 'bg-gray-50 dark:bg-gray-800/60 text-gray-600 dark:text-gray-400'}`}>
                    {agent.provider}
                  </span>
                  {agent.protected && (
                    <span className="text-[11px] px-2 py-0.5 rounded-full font-medium bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300">
                      Protected
                    </span>
                  )}
                </div>
                {agent.description && (
                  <p className="text-xs text-gray-500 dark:text-gray-400 truncate">{agent.description}</p>
                )}
                <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">{agent.model}</p>
                {agent.name === 'Code Review Agent' && (
                  <p className="text-[11px] text-gray-400 dark:text-gray-500 mt-1 italic">
                    Used as a prompt template by repo sessions — not run as a chat agent.
                  </p>
                )}
              </div>

              <div className="flex items-center gap-1 ml-4">
                <Link href={`/playground?agent=${agent.id}`}>
                  <button className="p-1.5 text-gray-400 dark:text-gray-500 hover:text-purple-600 hover:bg-purple-50 rounded-lg" title="Chat">
                    <MessageSquare size={15} />
                  </button>
                </Link>
                <Link href={`/agents/${agent.id}/edit`}>
                  <button className="p-1.5 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800 rounded-lg" title="Edit">
                    <Pencil size={15} />
                  </button>
                </Link>
                <button
                  onClick={() => exportAgent(agent)}
                  className="p-1.5 text-gray-400 dark:text-gray-500 hover:text-blue-600 hover:bg-blue-50 rounded-lg"
                  title="Export"
                >
                  <Download size={15} />
                </button>
                {!agent.protected && (
                  <button
                    onClick={() => { if (confirm('Delete this agent?')) deleteMutation.mutate(agent.id) }}
                    className="p-1.5 text-gray-400 dark:text-gray-500 hover:text-red-500 hover:bg-red-50 rounded-lg"
                    title="Delete"
                  >
                    <Trash2 size={15} />
                  </button>
                )}
              </div>
            </div>
          </div>
        ))}
      </div>

      <div className="mt-6 p-4 rounded-lg bg-gray-50 dark:bg-gray-800/60 border border-gray-200 dark:border-gray-700">
        <p className="text-sm text-gray-600 dark:text-gray-400">
          Learn how to build and configure agents in the{' '}
          <Link href="/docs/what-is-an-agent" className="text-purple-600 dark:text-purple-300 hover:underline">
            documentation
          </Link>.
        </p>
      </div>
    </div>
  )
}
