'use client'

import { useEffect, useState } from 'react'
import { useRouter, useParams } from 'next/navigation'
import { ChevronRight, Save, X } from 'lucide-react'
import { skillsAPI } from '@/lib/api'
import { ToolPicker } from '@/components/ToolPicker'
import { CATEGORIES } from '@/lib/tool-category'
import { Field } from '@/components/ui/Field'

export default function EditSkillPage() {
  const router = useRouter()
  const { id } = useParams<{ id: string }>()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [content, setContent] = useState('')
  const [category, setCategory] = useState('')
  const [requiredTools, setRequiredTools] = useState<string[]>([])
  const [enabled, setEnabled] = useState(true)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    skillsAPI.get(id)
      .then((s: any) => {
        setName(s.name ?? '')
        setDescription(s.description ?? '')
        setContent(s.content ?? '')
        setCategory(s.category ?? '')
        setRequiredTools(s.required_tool_names ?? [])
        setEnabled(s.enabled ?? true)
      })
      .catch(() => setError('Failed to load skill'))
      .finally(() => setLoading(false))
  }, [id])

  const save = async () => {
    if (!name.trim()) {
      setError('Skill name is required')
      return
    }
    await skillsAPI.update(id, { name, description, content, category, enabled, required_tool_names: requiredTools })
    router.push('/skills')
  }

  if (loading) return <div className="p-6 text-sm text-faint">Loading…</div>

  return (
    <div className="p-4 sm:p-6 max-w-3xl">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-5">
        <div className="flex items-center gap-2 text-[12px] text-faint">
          <span onClick={() => router.push('/skills')} className="hover:text-gray-600 dark:hover:text-gray-300 cursor-pointer">Skills</span>
          <ChevronRight className="w-3 h-3" />
          <span className="text-foreground font-medium">Edit skill</span>
        </div>
        <div className="flex gap-2">
          <button onClick={() => router.push('/skills')} className="inline-flex items-center gap-1.5 px-3 py-1.5 border border-border-strong text-muted-foreground text-[12px] rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800">
            <X className="w-3.5 h-3.5" /> Discard
          </button>
          <button onClick={save} className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-accent text-white text-[12px] rounded-lg hover:bg-accent-hover">
            <Save className="w-3.5 h-3.5" /> Save Skill
          </button>
        </div>
      </div>
      {error && <div className="mb-4 text-sm text-crit bg-crit/10 border border-crit/30 rounded-lg px-3 py-2">{error}</div>}
      <div className="space-y-4">
        <Field label="Name"><input value={name} onChange={(e) => setName(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg" /></Field>
        <Field label="Description"><input value={description} onChange={(e) => setDescription(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg" /></Field>
        <Field label="Category">
          <select value={category} onChange={(e) => setCategory(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg bg-surface">
            <option value="">Uncategorized</option>
            {CATEGORIES.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        </Field>
        <Field label="Instructions"><textarea value={content} onChange={(e) => setContent(e.target.value)} rows={12} className="w-full text-[13px] px-3 py-2 border border-border-strong rounded-lg font-mono" /></Field>
        <Field label="Required tools">
          <ToolPicker selected={requiredTools} onChange={setRequiredTools} />
          <p className="text-[11px] text-faint mt-1">Pick the tools this skill needs. With lazy tool loading, they are auto-activated when this skill is requested.</p>
        </Field>
        <label className="flex items-center gap-2 text-sm text-foreground">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          Enabled
        </label>
      </div>
    </div>
  )
}

