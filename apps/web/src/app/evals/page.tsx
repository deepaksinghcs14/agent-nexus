'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { agentsAPI, evalsAPI } from '@/lib/api'
import type { Agent, EvalSuite } from '@/types'
import { FlaskConical, Plus, Trash2, ChevronRight } from 'lucide-react'

const GRADING_LABELS: Record<string, string> = {
  llm_judge: 'LLM Judge',
  exact: 'Exact Match',
  contains: 'Contains',
}

function ScoreBadge({ score }: { score?: number }) {
  if (score == null) return <span className="text-xs text-faint">No runs</span>
  const pct = Math.round(score * 100)
  const color = pct >= 80 ? 'text-green-600 dark:text-green-300 bg-green-50 dark:bg-green-500/10' : pct >= 50 ? 'text-amber-600 dark:text-amber-300 bg-amber-50 dark:bg-amber-500/10' : 'text-red-600 dark:text-red-300 bg-red-50 dark:bg-red-500/10'
  return <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${color}`}>{pct}%</span>
}

function CreateSuiteModal({ agents, onCreated, onClose }: {
  agents: Agent[]
  onCreated: () => void
  onClose: () => void
}) {
  const [name, setName] = useState('')
  const [desc, setDesc] = useState('')
  const [agentId, setAgentId] = useState(agents[0]?.id ?? '')
  const [mode, setMode] = useState('llm_judge')
  const [saving, setSaving] = useState(false)

  const submit = async () => {
    if (!name || !agentId) return
    setSaving(true)
    await evalsAPI.createSuite({ agent_id: agentId, name, description: desc, grading_mode: mode }).catch(() => {})
    setSaving(false)
    onCreated()
    onClose()
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
      <div className="bg-surface rounded-lg shadow-xl w-full max-w-lg mx-4">
        <div className="px-6 py-4 border-b">
          <h2 className="text-base font-semibold text-foreground">New eval suite</h2>
        </div>
        <div className="px-6 py-4 space-y-4">
          <div>
            <label className="block text-sm font-medium text-foreground mb-1">Name</label>
            <input
              className="w-full border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-accent"
              value={name} onChange={e => setName(e.target.value)} placeholder="e.g. Customer Support QA"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-foreground mb-1">Agent</label>
            <select
              className="w-full border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-accent"
              value={agentId} onChange={e => setAgentId(e.target.value)}
            >
              {agents.map(a => <option key={a.id} value={a.id}>{a.name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-foreground mb-1">Grading mode</label>
            <select
              className="w-full border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-accent"
              value={mode} onChange={e => setMode(e.target.value)}
            >
              <option value="llm_judge">LLM Judge — model grades each response</option>
              <option value="contains">Contains — output must include expected text</option>
              <option value="exact">Exact Match — output must match exactly</option>
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-foreground mb-1">Description (optional)</label>
            <textarea
              className="w-full border rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-accent resize-none"
              rows={2} value={desc} onChange={e => setDesc(e.target.value)}
            />
          </div>
        </div>
        <div className="px-6 py-4 border-t flex justify-end gap-3">
          <button onClick={onClose} className="px-4 py-2 text-sm text-muted-foreground hover:text-gray-900 transition-colors">Cancel</button>
          <button
            onClick={submit} disabled={saving || !name || !agentId}
            className="px-4 py-2 bg-accent hover:bg-accent-hover text-white text-sm font-medium rounded-md transition-colors disabled:opacity-50"
          >
            {saving ? 'Creating…' : 'Create suite'}
          </button>
        </div>
      </div>
    </div>
  )
}

export default function EvalsPage() {
  const [suites, setSuites] = useState<EvalSuite[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)

  const load = () => {
    Promise.all([
      evalsAPI.listSuites().then((r: unknown) => setSuites(((r as { data?: EvalSuite[] }).data) ?? [])).catch(() => {}),
      agentsAPI.list().then((r: unknown) => setAgents(((r as { data?: Agent[] }).data) ?? [])).catch(() => {}),
    ]).finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this eval suite and all its cases and run history?')) return
    await evalsAPI.deleteSuite(id).catch(() => {})
    load()
  }

  return (
    <div className="p-4 sm:p-6">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-8">
        <div>
          <h1 className="text-xl font-semibold text-foreground">Evals</h1>
          <p className="text-sm text-muted-foreground mt-1">Test suites that run your agents against known inputs and grade their outputs.</p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="flex items-center gap-2 px-4 py-2 rounded-md bg-accent hover:bg-accent-hover text-white text-sm font-medium transition-colors"
        >
          <Plus className="w-4 h-4" />
          New suite
        </button>
      </div>

      {loading ? (
        <div className="py-10 text-center text-sm text-faint">Loading…</div>
      ) : suites.length === 0 ? (
        <div className="py-16 text-center">
          <FlaskConical className="w-10 h-10 text-faint mx-auto mb-3" />
          <p className="text-sm font-medium text-muted-foreground">No eval suites yet</p>
          <p className="text-sm text-faint mt-1">Create a suite to start testing your agents systematically.</p>
          <button
            onClick={() => setShowCreate(true)}
            className="mt-4 px-4 py-2 bg-accent hover:bg-accent-hover text-white text-sm font-medium rounded-md transition-colors"
          >
            Create your first suite
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {suites.map(s => (
            <div key={s.id} className="bg-surface border border-border-strong rounded-lg p-4 hover:border-gray-300 transition-colors group">
              <div className="flex items-start justify-between gap-2 mb-3">
                <div className="min-w-0 flex-1">
                  <Link href={`/evals/${s.id}`} className="text-sm font-medium text-foreground hover:text-purple-700 transition-colors line-clamp-1">
                    {s.name}
                  </Link>
                  <p className="text-xs text-muted-foreground mt-0.5">{s.agent_name}</p>
                </div>
                <ScoreBadge score={s.last_score} />
              </div>
              {s.description && (
                <p className="text-xs text-muted-foreground line-clamp-2 mb-3">{s.description}</p>
              )}
              <div className="flex items-center justify-between text-xs text-faint">
                <div className="flex items-center gap-3">
                  <span>{s.case_count ?? 0} cases</span>
                  <span>{GRADING_LABELS[s.grading_mode] ?? s.grading_mode}</span>
                </div>
                <div className="flex items-center gap-1">
                  <button
                    onClick={() => handleDelete(s.id)}
                    className="p-1 rounded hover:bg-red-50 hover:text-red-500 transition-colors opacity-0 group-hover:opacity-100"
                    title="Delete suite"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                  <Link href={`/evals/${s.id}`} className="p-1 rounded hover:bg-muted transition-colors">
                    <ChevronRight className="w-3.5 h-3.5" />
                  </Link>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {showCreate && agents.length > 0 && (
        <CreateSuiteModal agents={agents} onCreated={load} onClose={() => setShowCreate(false)} />
      )}
      {showCreate && agents.length === 0 && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
          <div className="bg-surface rounded-lg shadow-xl w-full max-w-sm mx-4 p-6 text-center">
            <p className="text-sm text-foreground mb-4">You need at least one agent before creating an eval suite.</p>
            <div className="flex gap-3 justify-center">
              <button onClick={() => setShowCreate(false)} className="px-4 py-2 text-sm text-muted-foreground hover:text-gray-900 transition-colors">Close</button>
              <Link href="/agents" className="px-4 py-2 bg-accent hover:bg-accent-hover text-white text-sm font-medium rounded-md transition-colors">Create agent</Link>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
