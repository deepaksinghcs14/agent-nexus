'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { BookMarked, Lock, Pencil, Plus, Search, Trash2 } from 'lucide-react'
import { skillsAPI } from '@/lib/api'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable, Row, Cell } from '@/components/ui/DataTable'
import type { Skill } from '@/types'

export default function SkillsPage() {
  const [skills, setSkills] = useState<Skill[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')

  const load = () => {
    skillsAPI.list()
      .then((r) => setSkills(((r as { data?: Skill[] }).data) ?? []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }
  useEffect(() => { load() }, [])

  const remove = async (s: Skill) => {
    if (s.source === 'managed') return
    if (!confirm('Delete this skill? Agents using it will lose these instructions.')) return
    await skillsAPI.delete(s.id).catch(() => {})
    load()
  }

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return skills
    return skills.filter((s) =>
      s.name.toLowerCase().includes(q) ||
      (s.description ?? '').toLowerCase().includes(q) ||
      (s.category ?? '').toLowerCase().includes(q))
  }, [skills, search])

  return (
    <div className="p-4 sm:p-6 max-w-6xl">
      <PageHeader
        eyebrow="Integrations"
        title="Skills"
        subtitle="Reusable instruction blocks with the tools they need — attach them to any agent."
        actions={
          <Link href="/skills/new" className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-[10px] bg-gradient-to-br from-accent to-accent-ink hover:opacity-95 text-white text-[13px] font-semibold shadow-card">
            <Plus className="w-4 h-4" /> New skill
          </Link>
        }
      />

      {!loading && skills.length > 0 && (
        <div className="flex items-center gap-2 mb-4 px-3 py-2 border border-border rounded-[10px] bg-surface max-w-sm">
          <Search className="w-3.5 h-3.5 text-faint" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search skills…" className="flex-1 text-[13px] bg-transparent outline-none" />
        </div>
      )}

      {loading ? (
        <div className="py-10 text-center text-sm text-faint">Loading…</div>
      ) : skills.length === 0 ? (
        <div className="border border-dashed border-border-strong rounded-2xl py-16 text-center bg-surface">
          <BookMarked className="w-10 h-10 text-faint mx-auto mb-3" />
          <p className="text-sm font-medium text-muted-foreground mb-1">No skills yet</p>
          <p className="text-sm text-faint mb-4">Create reusable instructions for agents and channels.</p>
          <Link href="/skills/new" className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-accent hover:bg-accent-hover text-white text-sm font-medium">
            <Plus className="w-4 h-4" /> Create skill
          </Link>
        </div>
      ) : (
        <DataTable columns={['Skill', 'Category', 'Source', 'Required tools', '']} minWidth={720}>
          {filtered.map((s) => (
            <Row key={s.id}>
              <Cell className="!whitespace-normal">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-foreground">{s.name}</span>
                  {!s.enabled && <span className="font-mono text-[10px] px-1.5 py-0.5 rounded-full bg-muted text-muted-foreground">disabled</span>}
                </div>
                <div className="text-xs text-faint mt-0.5 line-clamp-1 max-w-md">{s.description || 'No description'}</div>
              </Cell>
              <Cell>
                {s.category
                  ? <span className="font-mono text-[10px] px-2 py-0.5 rounded-full bg-accent/10 text-accent dark:text-accent-bright border border-accent/25">{s.category}</span>
                  : <span className="font-mono text-[10px] text-faint">—</span>}
              </Cell>
              <Cell>
                <span className={`font-mono text-[10px] px-2 py-0.5 rounded-full border ${s.source === 'managed' ? 'border-warn/40 text-warn' : 'border-border text-muted-foreground'}`}>{s.source}</span>
              </Cell>
              <Cell className="!whitespace-normal">
                {s.required_tool_names && s.required_tool_names.length > 0 ? (
                  <div className="flex flex-wrap gap-1 max-w-xs">
                    {s.required_tool_names.slice(0, 3).map((t) => (
                      <span key={t} className="font-mono text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground">{t}</span>
                    ))}
                    {s.required_tool_names.length > 3 && <span className="font-mono text-[10px] text-faint">+{s.required_tool_names.length - 3}</span>}
                  </div>
                ) : <span className="font-mono text-[11px] text-faint">—</span>}
              </Cell>
              <Cell className="text-right">
                {s.source === 'managed' ? (
                  <Lock className="w-4 h-4 text-faint inline" />
                ) : (
                  <span className="inline-flex items-center gap-0.5">
                    <Link href={`/skills/${s.id}/edit`} className="p-1.5 rounded text-faint hover:text-accent dark:hover:text-accent-bright hover:bg-accent/10" title="Edit"><Pencil className="w-4 h-4" /></Link>
                    <button onClick={() => remove(s)} className="p-1.5 rounded text-faint hover:text-crit hover:bg-crit/10" title="Delete"><Trash2 className="w-4 h-4" /></button>
                  </span>
                )}
              </Cell>
            </Row>
          ))}
        </DataTable>
      )}
    </div>
  )
}
