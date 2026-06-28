'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { BookMarked, Lock, Pencil, Plus, Trash2 } from 'lucide-react'
import { skillsAPI } from '@/lib/api'
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
    <div className="p-4 sm:p-6">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-8">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Skills</h1>
          <p className="text-sm text-gray-500 mt-1">Reusable prompt instructions that can be attached to agents.</p>
        </div>
        <Link href="/skills/new" className="flex items-center gap-2 px-4 py-2 rounded-md bg-purple-600 hover:bg-purple-700 text-white text-sm font-medium">
          <Plus className="w-4 h-4" /> New skill
        </Link>
      </div>
      {loading ? (
        <div className="py-10 text-center text-sm text-gray-400">Loading…</div>
      ) : skills.length === 0 ? (
        <div className="py-16 text-center">
          <BookMarked className="w-10 h-10 text-gray-300 mx-auto mb-3" />
          <p className="text-sm font-medium text-gray-600 mb-1">No skills yet</p>
          <p className="text-sm text-gray-400 mb-4">Create reusable instructions for agents and channels.</p>
          <Link href="/skills/new" className="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-purple-600 hover:bg-purple-700 text-white text-sm font-medium">
            <Plus className="w-4 h-4" /> Create skill
          </Link>
        </div>
      ) : (
        <div className="rounded-xl border border-gray-100 overflow-hidden divide-y divide-gray-100">
          {skills.map((s) => (
            <div key={s.id} className="bg-white px-4 sm:px-5 py-4 flex items-start justify-between gap-4">
              <div className="min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <span className="text-sm font-medium text-gray-900">{s.name}</span>
                  <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${s.source === 'managed' ? 'bg-amber-50 text-amber-700' : 'bg-blue-50 text-blue-700'}`}>{s.source}</span>
                  {!s.enabled && <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-gray-100 text-gray-500">disabled</span>}
                </div>
                <p className="text-xs text-gray-500 mb-2">{s.description || 'No description'}</p>
                <p className="text-xs text-gray-400 line-clamp-2 max-w-3xl">{s.content}</p>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                {s.source === 'managed' ? (
                  <Lock className="w-4 h-4 text-gray-300" />
                ) : (
                  <>
                    <Link href={`/skills/${s.id}/edit`} className="p-1.5 rounded text-gray-400 hover:text-purple-600 hover:bg-purple-50" title="Edit">
                      <Pencil className="w-4 h-4" />
                    </Link>
                    <button onClick={() => remove(s)} className="p-1.5 rounded text-gray-400 hover:text-red-500 hover:bg-red-50" title="Delete">
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
