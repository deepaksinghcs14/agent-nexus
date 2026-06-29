'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { use } from 'react'
import { evalsAPI } from '@/lib/api'
import type { EvalCase, EvalRun, EvalSuite } from '@/types'
import { Plus, Trash2, Play, ChevronRight, Pencil, X, Check } from 'lucide-react'

function RunStatusBadge({ status }: { status: EvalRun['status'] }) {
  const cls = {
    pending: 'bg-gray-100 text-gray-500',
    running: 'bg-blue-50 text-blue-600',
    completed: 'bg-green-50 text-green-700',
    failed: 'bg-red-50 text-red-600',
  }[status] ?? 'bg-gray-100 text-gray-500'
  return <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${cls}`}>{status}</span>
}

function ScorePct({ score, status }: { score: number; status: string }) {
  if (status === 'pending' || status === 'running') return <span className="text-xs text-gray-400">—</span>
  const pct = Math.round(score * 100)
  const color = pct >= 80 ? 'text-green-600' : pct >= 50 ? 'text-amber-600' : 'text-red-600'
  return <span className={`text-sm font-semibold ${color}`}>{pct}%</span>
}

function CaseRow({ c, suiteId, onDeleted, onUpdated }: {
  c: EvalCase
  suiteId: string
  onDeleted: () => void
  onUpdated: () => void
}) {
  const [editing, setEditing] = useState(false)
  const [input, setInput] = useState(c.input)
  const [expected, setExpected] = useState(c.expected_output)
  const [criteria, setCriteria] = useState(c.grading_criteria)
  const [saving, setSaving] = useState(false)

  const save = async () => {
    setSaving(true)
    await evalsAPI.updateCase(suiteId, c.id, { input, expected_output: expected, grading_criteria: criteria }).catch(() => {})
    setSaving(false)
    setEditing(false)
    onUpdated()
  }

  if (editing) {
    return (
      <div className="border border-purple-200 rounded-lg p-4 space-y-3 bg-purple-50/30">
        <div>
          <label className="text-xs font-medium text-gray-600 block mb-1">Input</label>
          <textarea className="w-full border rounded px-3 py-2 text-sm resize-none focus:outline-none focus:ring-2 focus:ring-purple-500" rows={3} value={input} onChange={e => setInput(e.target.value)} />
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label className="text-xs font-medium text-gray-600 block mb-1">Expected output (optional)</label>
            <textarea className="w-full border rounded px-3 py-2 text-sm resize-none focus:outline-none focus:ring-2 focus:ring-purple-500" rows={2} value={expected} onChange={e => setExpected(e.target.value)} />
          </div>
          <div>
            <label className="text-xs font-medium text-gray-600 block mb-1">Grading criteria (optional)</label>
            <textarea className="w-full border rounded px-3 py-2 text-sm resize-none focus:outline-none focus:ring-2 focus:ring-purple-500" rows={2} value={criteria} onChange={e => setCriteria(e.target.value)} />
          </div>
        </div>
        <div className="flex justify-end gap-2">
          <button onClick={() => setEditing(false)} className="p-1.5 rounded hover:bg-gray-100 text-gray-500 transition-colors"><X className="w-4 h-4" /></button>
          <button onClick={save} disabled={saving || !input} className="p-1.5 rounded hover:bg-green-100 text-green-600 transition-colors disabled:opacity-50"><Check className="w-4 h-4" /></button>
        </div>
      </div>
    )
  }

  return (
    <div className="border border-gray-100 rounded-lg p-4 group hover:border-gray-200 transition-colors">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0 flex-1 space-y-1">
          <p className="text-sm text-gray-900 line-clamp-2">{c.input}</p>
          {c.expected_output && <p className="text-xs text-gray-500"><span className="font-medium">Expected:</span> {c.expected_output}</p>}
          {c.grading_criteria && <p className="text-xs text-gray-500"><span className="font-medium">Criteria:</span> {c.grading_criteria}</p>}
        </div>
        <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0">
          <button onClick={() => setEditing(true)} className="p-1 rounded hover:bg-gray-100 text-gray-400 hover:text-gray-600 transition-colors"><Pencil className="w-3.5 h-3.5" /></button>
          <button onClick={onDeleted} className="p-1 rounded hover:bg-red-50 text-gray-400 hover:text-red-500 transition-colors"><Trash2 className="w-3.5 h-3.5" /></button>
        </div>
      </div>
    </div>
  )
}

function AddCaseForm({ suiteId, onCreated }: { suiteId: string; onCreated: () => void }) {
  const [open, setOpen] = useState(false)
  const [input, setInput] = useState('')
  const [expected, setExpected] = useState('')
  const [criteria, setCriteria] = useState('')
  const [saving, setSaving] = useState(false)

  const submit = async () => {
    if (!input) return
    setSaving(true)
    await evalsAPI.createCase(suiteId, { input, expected_output: expected, grading_criteria: criteria }).catch(() => {})
    setSaving(false)
    setInput('')
    setExpected('')
    setCriteria('')
    setOpen(false)
    onCreated()
  }

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="w-full border-2 border-dashed border-gray-200 rounded-lg py-3 text-sm text-gray-400 hover:border-purple-300 hover:text-purple-500 transition-colors flex items-center justify-center gap-2"
      >
        <Plus className="w-4 h-4" /> Add test case
      </button>
    )
  }

  return (
    <div className="border border-purple-200 rounded-lg p-4 space-y-3 bg-purple-50/30">
      <div>
        <label className="text-xs font-medium text-gray-600 block mb-1">Input <span className="text-red-500">*</span></label>
        <textarea className="w-full border rounded px-3 py-2 text-sm resize-none focus:outline-none focus:ring-2 focus:ring-purple-500" rows={3} value={input} onChange={e => setInput(e.target.value)} placeholder="The message to send to the agent" />
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <div>
          <label className="text-xs font-medium text-gray-600 block mb-1">Expected output <span className="text-gray-400 font-normal">(optional)</span></label>
          <textarea className="w-full border rounded px-3 py-2 text-sm resize-none focus:outline-none focus:ring-2 focus:ring-purple-500" rows={2} value={expected} onChange={e => setExpected(e.target.value)} placeholder="Reference answer" />
        </div>
        <div>
          <label className="text-xs font-medium text-gray-600 block mb-1">Grading criteria <span className="text-gray-400 font-normal">(optional)</span></label>
          <textarea className="w-full border rounded px-3 py-2 text-sm resize-none focus:outline-none focus:ring-2 focus:ring-purple-500" rows={2} value={criteria} onChange={e => setCriteria(e.target.value)} placeholder="What should the LLM judge look for?" />
        </div>
      </div>
      <div className="flex justify-end gap-2">
        <button onClick={() => setOpen(false)} className="px-3 py-1.5 text-sm text-gray-600 hover:text-gray-900 transition-colors">Cancel</button>
        <button onClick={submit} disabled={saving || !input} className="px-3 py-1.5 bg-purple-600 hover:bg-purple-700 text-white text-sm font-medium rounded-md transition-colors disabled:opacity-50">
          {saving ? 'Adding…' : 'Add case'}
        </button>
      </div>
    </div>
  )
}

export default function SuitePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const [suite, setSuite] = useState<EvalSuite | null>(null)
  const [cases, setCases] = useState<EvalCase[]>([])
  const [runs, setRuns] = useState<EvalRun[]>([])
  const [tab, setTab] = useState<'cases' | 'runs'>('cases')
  const [loading, setLoading] = useState(true)
  const [triggering, setTriggering] = useState(false)

  const loadSuite = () =>
    evalsAPI.getSuite(id).then((r: unknown) => {
      const res = r as { suite: EvalSuite; cases: EvalCase[] }
      setSuite(res.suite)
      setCases(res.cases)
    }).catch(() => {})

  const loadRuns = () =>
    evalsAPI.listRuns(id).then((r: unknown) => setRuns(((r as { data?: EvalRun[] }).data) ?? [])).catch(() => {})

  useEffect(() => {
    Promise.all([loadSuite(), loadRuns()]).finally(() => setLoading(false))
  }, [id])

  // Poll while any run is active
  useEffect(() => {
    const active = runs.some(r => r.status === 'pending' || r.status === 'running')
    if (!active) return
    const t = setInterval(loadRuns, 2000)
    return () => clearInterval(t)
  }, [runs])

  const handleDeleteCase = async (caseId: string) => {
    if (!confirm('Delete this test case?')) return
    await evalsAPI.deleteCase(id, caseId).catch(() => {})
    loadSuite()
  }

  const handleTriggerRun = async () => {
    setTriggering(true)
    await evalsAPI.triggerRun(id).catch(() => {})
    setTriggering(false)
    setTab('runs')
    loadRuns()
  }

  if (loading) return <div className="p-6 text-sm text-gray-400">Loading…</div>
  if (!suite) return <div className="p-6 text-sm text-gray-500">Suite not found.</div>

  return (
    <div className="p-4 sm:p-6">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-6">
        <div>
          <div className="flex items-center gap-2 text-sm text-gray-400 mb-1">
            <Link href="/evals" className="hover:text-gray-600 transition-colors">Evals</Link>
            <span>/</span>
            <span className="text-gray-600">{suite.name}</span>
          </div>
          <h1 className="text-xl font-semibold text-gray-900">{suite.name}</h1>
          <p className="text-sm text-gray-500 mt-0.5">{suite.agent_name} · {suite.grading_mode === 'llm_judge' ? 'LLM Judge' : suite.grading_mode === 'contains' ? 'Contains' : 'Exact Match'}</p>
        </div>
        <button
          onClick={handleTriggerRun}
          disabled={triggering || cases.length === 0}
          className="flex items-center gap-2 px-4 py-2 bg-purple-600 hover:bg-purple-700 text-white text-sm font-medium rounded-md transition-colors disabled:opacity-50"
        >
          <Play className="w-4 h-4" />
          {triggering ? 'Starting…' : 'Run eval'}
        </button>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 border-b border-gray-200 mb-6 overflow-x-auto whitespace-nowrap">
        {(['cases', 'runs'] as const).map(t => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${tab === t ? 'border-purple-600 text-purple-600' : 'border-transparent text-gray-500 hover:text-gray-700'}`}
          >
            {t === 'cases' ? `Cases (${cases.length})` : `Run history (${runs.length})`}
          </button>
        ))}
      </div>

      {tab === 'cases' && (
        <div className="space-y-3 max-w-3xl">
          {cases.map(c => (
            <CaseRow
              key={c.id}
              c={c}
              suiteId={id}
              onDeleted={() => handleDeleteCase(c.id)}
              onUpdated={loadSuite}
            />
          ))}
          <AddCaseForm suiteId={id} onCreated={loadSuite} />
        </div>
      )}

      {tab === 'runs' && (
        <div>
          {runs.length === 0 ? (
            <div className="py-12 text-center">
              <p className="text-sm text-gray-400">No runs yet. Add test cases and click &ldquo;Run eval&rdquo;.</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="min-w-[540px] w-full text-sm">
                <thead>
                  <tr className="border-b text-xs text-gray-500 uppercase tracking-wide">
                    <th className="text-left py-2 pr-4 font-medium">Started</th>
                    <th className="text-left py-2 pr-4 font-medium">Status</th>
                    <th className="text-left py-2 pr-4 font-medium">Score</th>
                    <th className="text-left py-2 pr-4 font-medium">Cases</th>
                    <th className="text-left py-2 font-medium">Duration</th>
                    <th />
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {runs.map(r => {
                    const dur = r.started_at && r.completed_at
                      ? Math.round((new Date(r.completed_at).getTime() - new Date(r.started_at).getTime()) / 1000) + 's'
                      : '—'
                    return (
                      <tr key={r.id} className="hover:bg-gray-50 transition-colors">
                        <td className="py-3 pr-4 text-gray-600">{new Date(r.created_at).toLocaleString()}</td>
                        <td className="py-3 pr-4"><RunStatusBadge status={r.status} /></td>
                        <td className="py-3 pr-4"><ScorePct score={r.score} status={r.status} /></td>
                        <td className="py-3 pr-4 text-gray-500">
                          {r.status !== 'pending' ? `${r.passed}/${r.total_cases} passed` : `${r.total_cases} cases`}
                        </td>
                        <td className="py-3 text-gray-400">{dur}</td>
                        <td className="py-3 pl-2">
                          <Link href={`/evals/${id}/runs/${r.id}`} className="text-purple-600 hover:text-purple-700 transition-colors">
                            <ChevronRight className="w-4 h-4" />
                          </Link>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
