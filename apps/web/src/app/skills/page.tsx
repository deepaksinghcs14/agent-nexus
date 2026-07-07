'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { BookMarked, Lock, Pencil, Plus, Trash2 } from 'lucide-react'
import { skillsAPI } from '@/lib/api'
import { PageHeader } from '@/components/ui/PageHeader'
import type { Skill } from '@/types'

export default function SkillsPage() {
  const [skills, setSkills] = useState<Skill[]>([])
  const [loading, setLoading] = useState(true)

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
      {loading ? (
        <div className="py-10 text-center text-sm text-faint">Loading…</div>
      ) : skills.length === 0 ? (
        <div className="py-16 text-center">
          <BookMarked className="w-10 h-10 text-faint mx-auto mb-3" />
          <p className="text-sm font-medium text-muted-foreground mb-1">No skills yet</p>
          <p className="text-sm text-faint mb-4">Create reusable instructions for agents and channels.</p>
          <Link href="/skills/new" className="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-accent hover:bg-accent-hover text-white text-sm font-medium">
            <Plus className="w-4 h-4" /> Create skill
          </Link>
        </div>
      ) : (
        <div className="rounded-xl border border-border overflow-hidden divide-y divide-border">
          {skills.map((s) => (
            <div key={s.id} className="bg-surface px-4 sm:px-5 py-4 flex items-start justify-between gap-4">
              <div className="min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <span className="text-sm font-medium text-foreground">{s.name}</span>
                  <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${s.source === 'managed' ? 'bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-300' : 'bg-blue-50 dark:bg-blue-500/10 text-blue-700 dark:text-blue-300'}`}>{s.source}</span>
                  {!s.enabled && <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-muted text-muted-foreground">disabled</span>}
                </div>
                <p className="text-xs text-muted-foreground mb-2">{s.description || 'No description'}</p>
                <p className="text-xs text-faint line-clamp-2 max-w-3xl">{s.content}</p>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                {s.source === 'managed' ? (
                  <Lock className="w-4 h-4 text-faint" />
                ) : (
                  <>
                    <Link href={`/skills/${s.id}/edit`} className="p-1.5 rounded text-faint hover:text-accent dark:text-accent-bright hover:bg-accent/10" title="Edit">
                      <Pencil className="w-4 h-4" />
                    </Link>
                    <button onClick={() => remove(s)} className="p-1.5 rounded text-faint hover:text-red-500 hover:bg-red-50" title="Delete">
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
