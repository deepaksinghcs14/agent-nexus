'use client'

import { useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { evalsAPI } from '@/lib/api'
import type { EvalCase, EvalRun, EvalSuite } from '@/types'
import { Plus, Trash2, Play, ChevronRight, Pencil, X, Check, Sparkles, Download, Upload, ToggleLeft, ToggleRight } from 'lucide-react'

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

function GenerateCasesModal({ suiteId, suiteName, onDone }: { suiteId: string; suiteName: string; onDone: () => void }) {
  const [count, setCount] = useState(10)
  const [loading, setLoading] = useState(false)
  const [generated, setGenerated] = useState<EvalCase[] | null>(null)
  const [error, setError] = useState('')

  const generate = async () => {
    setLoading(true)
    setError('')
    setGenerated(null)
    try {
      const res = await evalsAPI.generateCases(suiteId, count) as { cases: EvalCase[]; count: number }
      setGenerated(res.cases ?? [])
    } catch {
      setError('Generation failed. Make sure the agent has a configured LLM provider.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[90vh] flex flex-col">
        <div className="flex items-center justify-between p-4 border-b">
          <div className="flex items-center gap-2">
            <Sparkles className="w-4 h-4 text-purple-500" />
            <h2 className="font-semibold text-gray-900 text-sm">Generate test cases</h2>
          </div>
          <button onClick={onDone} className="p-1 rounded hover:bg-gray-100 text-gray-400"><X className="w-4 h-4" /></button>
        </div>

        <div className="p-4 space-y-4 overflow-y-auto flex-1">
          {!generated ? (
            <>
              <p className="text-sm text-gray-500">
                The AI will analyse <strong>{suiteName}</strong>&apos;s system prompt, tools, skills, and knowledge bases to suggest realistic test cases.
              </p>
              <div>
                <label className="text-xs font-medium text-gray-600 block mb-2">Number of cases to generate</label>
                <div className="flex gap-2">
                  {[5, 10, 15, 20].map(n => (
                    <button
                      key={n}
                      onClick={() => setCount(n)}
                      className={`px-3 py-1.5 rounded-md text-sm font-medium border transition-colors ${count === n ? 'bg-purple-600 text-white border-purple-600' : 'border-gray-200 text-gray-600 hover:border-purple-300'}`}
                    >
                      {n}
                    </button>
                  ))}
                </div>
              </div>
              {error && <p className="text-sm text-red-600">{error}</p>}
            </>
          ) : (
            <>
              <p className="text-sm text-green-600 font-medium">{generated.length} cases generated</p>
              <div className="space-y-2">
                {generated.map((c, i) => (
                  <div key={i} className="border border-gray-100 rounded-lg p-3 text-xs space-y-1">
                    <p className="text-gray-900 font-medium line-clamp-2">{c.input}</p>
                    {c.grading_criteria && <p className="text-gray-400 line-clamp-1"><span className="font-medium">Criteria:</span> {c.grading_criteria}</p>}
                  </div>
                ))}
              </div>
            </>
          )}
        </div>

        <div className="p-4 border-t flex justify-end gap-2">
          <button onClick={onDone} className="px-3 py-1.5 text-sm text-gray-600 hover:text-gray-900 transition-colors">
            {generated ? 'Done' : 'Cancel'}
          </button>
          {!generated && (
            <button
              onClick={generate}
              disabled={loading}
              className="flex items-center gap-2 px-4 py-1.5 bg-purple-600 hover:bg-purple-700 text-white text-sm font-medium rounded-md transition-colors disabled:opacity-50"
            >
              <Sparkles className="w-3.5 h-3.5" />
              {loading ? `Generating ${count} cases…` : 'Generate'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

function ImportModal({ suiteId, onDone }: { suiteId: string; onDone: () => void }) {
  const [text, setText] = useState('')
  const [importing, setImporting] = useState(false)
  const [result, setResult] = useState<{ imported: number } | null>(null)
  const [error, setError] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)

  let parsed: { input: string; expected_output?: string; grading_criteria?: string }[] | null = null
  let parseError = ''
  if (text.trim()) {
    try {
      parsed = JSON.parse(text)
      if (!Array.isArray(parsed)) { parsed = null; parseError = 'Must be a JSON array' }
    } catch { parseError = 'Invalid JSON' }
  }

  const handleFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    if (!f) return
    const reader = new FileReader()
    reader.onload = ev => setText(ev.target?.result as string ?? '')
    reader.readAsText(f)
  }

  const doImport = async () => {
    if (!parsed) return
    setImporting(true)
    setError('')
    try {
      const res = await evalsAPI.importCases(suiteId, parsed) as { imported: number }
      setResult(res)
    } catch {
      setError('Import failed. Please try again.')
    } finally {
      setImporting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg mx-4 flex flex-col max-h-[90vh]">
        <div className="flex items-center justify-between p-4 border-b">
          <div className="flex items-center gap-2">
            <Upload className="w-4 h-4 text-gray-500" />
            <h2 className="font-semibold text-gray-900 text-sm">Import test cases</h2>
          </div>
          <button onClick={onDone} className="p-1 rounded hover:bg-gray-100 text-gray-400"><X className="w-4 h-4" /></button>
        </div>

        <div className="p-4 space-y-3 overflow-y-auto flex-1">
          {result ? (
            <p className="text-sm text-green-600 font-medium">{result.imported} cases imported successfully.</p>
          ) : (
            <>
              <p className="text-xs text-gray-500">Paste a JSON array or upload a file exported from this page.</p>
              <p className="text-xs text-gray-400 font-mono">{`[{"input":"...","expected_output":"...","grading_criteria":"..."}]`}</p>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => fileRef.current?.click()}
                  className="text-xs px-3 py-1.5 border border-gray-200 rounded-md hover:bg-gray-50 transition-colors text-gray-600"
                >
                  Choose file…
                </button>
                <span className="text-xs text-gray-400">or paste below</span>
                <input ref={fileRef} type="file" accept=".json" className="hidden" onChange={handleFile} />
              </div>
              <textarea
                className="w-full border rounded px-3 py-2 text-xs font-mono resize-y focus:outline-none focus:ring-2 focus:ring-purple-500 min-h-[120px]"
                placeholder='[{"input": "What is ...?", "grading_criteria": "Should mention ..."}]'
                value={text}
                onChange={e => setText(e.target.value)}
              />
              {text.trim() && (
                parseError
                  ? <p className="text-xs text-red-500">{parseError}</p>
                  : <p className="text-xs text-green-600">{parsed?.length} case{parsed?.length === 1 ? '' : 's'} ready to import</p>
              )}
              {error && <p className="text-xs text-red-600">{error}</p>}
            </>
          )}
        </div>

        <div className="p-4 border-t flex justify-end gap-2">
          <button onClick={onDone} className="px-3 py-1.5 text-sm text-gray-600 hover:text-gray-900 transition-colors">
            {result ? 'Done' : 'Cancel'}
          </button>
          {!result && (
            <button
              onClick={doImport}
              disabled={!parsed || parsed.length === 0 || importing}
              className="px-4 py-1.5 bg-purple-600 hover:bg-purple-700 text-white text-sm font-medium rounded-md transition-colors disabled:opacity-50"
            >
              {importing ? 'Importing…' : 'Import'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

export default function SuitePage({ params }: { params: { id: string } }) {
  const { id } = params
  const [suite, setSuite] = useState<EvalSuite | null>(null)
  const [cases, setCases] = useState<EvalCase[]>([])
  const [runs, setRuns] = useState<EvalRun[]>([])
  const [tab, setTab] = useState<'cases' | 'runs'>('cases')
  const [loading, setLoading] = useState(true)
  const [triggering, setTriggering] = useState(false)
  const [showGenerate, setShowGenerate] = useState(false)
  const [showImport, setShowImport] = useState(false)
  const [togglingAutoRun, setTogglingAutoRun] = useState(false)
  const [exporting, setExporting] = useState(false)

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

  const handleToggleAutoRun = async () => {
    if (!suite) return
    setTogglingAutoRun(true)
    await evalsAPI.updateSuite(id, {
      name: suite.name,
      description: suite.description,
      grading_mode: suite.grading_mode,
      auto_run: !suite.auto_run,
    }).catch(() => {})
    setTogglingAutoRun(false)
    loadSuite()
  }

  const handleExport = async () => {
    if (!suite) return
    setExporting(true)
    try {
      const token = typeof window !== 'undefined' ? localStorage.getItem('token') : null
      const apiUrl = process.env.NEXT_PUBLIC_API_URL ?? ''
      const res = await fetch(`${apiUrl}/api/v1/evals/suites/${id}/cases/export`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      })
      if (!res.ok) throw new Error('Export failed')
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `${suite.name.toLowerCase().replace(/\s+/g, '-')}-cases.json`
      a.click()
      URL.revokeObjectURL(url)
    } catch { /* silent */ } finally {
      setExporting(false)
    }
  }

  if (loading) return <div className="p-6 text-sm text-gray-400">Loading…</div>
  if (!suite) return <div className="p-6 text-sm text-gray-500">Suite not found.</div>

  return (
    <div className="p-4 sm:p-6">
      {showGenerate && (
        <GenerateCasesModal
          suiteId={id}
          suiteName={suite.agent_name ?? suite.name}
          onDone={() => { setShowGenerate(false); loadSuite() }}
        />
      )}
      {showImport && (
        <ImportModal
          suiteId={id}
          onDone={() => { setShowImport(false); loadSuite() }}
        />
      )}

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
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={handleToggleAutoRun}
            disabled={togglingAutoRun}
            title={suite.auto_run ? 'Auto-run on agent save (on — click to disable)' : 'Auto-run on agent save (off — click to enable)'}
            className={`flex items-center gap-1.5 px-3 py-2 rounded-md text-sm font-medium border transition-colors disabled:opacity-50 ${suite.auto_run ? 'bg-purple-50 text-purple-700 border-purple-200' : 'bg-white text-gray-500 border-gray-200 hover:border-gray-300'}`}
          >
            {suite.auto_run ? <ToggleRight className="w-4 h-4" /> : <ToggleLeft className="w-4 h-4" />}
            Auto-run
          </button>
          <button
            onClick={handleTriggerRun}
            disabled={triggering || cases.length === 0}
            className="flex items-center gap-2 px-4 py-2 bg-purple-600 hover:bg-purple-700 text-white text-sm font-medium rounded-md transition-colors disabled:opacity-50"
          >
            <Play className="w-4 h-4" />
            {triggering ? 'Starting…' : 'Run eval'}
          </button>
        </div>
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
        <div className="max-w-3xl space-y-4">
          {/* Cases toolbar */}
          <div className="flex flex-wrap items-center gap-2 justify-between">
            <span className="text-xs text-gray-400">{cases.length} case{cases.length === 1 ? '' : 's'}</span>
            <div className="flex flex-wrap items-center gap-2">
              <button
                onClick={() => setShowImport(true)}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-gray-200 rounded-md hover:bg-gray-50 text-gray-600 transition-colors"
              >
                <Upload className="w-3.5 h-3.5" /> Import
              </button>
              <button
                onClick={handleExport}
                disabled={exporting || cases.length === 0}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm border border-gray-200 rounded-md hover:bg-gray-50 text-gray-600 transition-colors disabled:opacity-50"
              >
                <Download className="w-3.5 h-3.5" /> {exporting ? 'Exporting…' : 'Export'}
              </button>
              <button
                onClick={() => setShowGenerate(true)}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-purple-50 hover:bg-purple-100 text-purple-700 border border-purple-200 rounded-md transition-colors font-medium"
              >
                <Sparkles className="w-3.5 h-3.5" /> Generate
              </button>
            </div>
          </div>

          <div className="space-y-3">
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
