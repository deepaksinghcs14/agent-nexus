'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { ChevronRight, Save, X } from 'lucide-react'
import { skillsAPI } from '@/lib/api'

export default function NewSkillPage() {
  const router = useRouter()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [content, setContent] = useState('')
  const [requiredTools, setRequiredTools] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [error, setError] = useState('')

  const save = async () => {
    if (!name.trim()) {
      setError('Skill name is required')
      return
    }
    const required_tool_names = requiredTools.split(',').map(s => s.trim()).filter(Boolean)
    await skillsAPI.create({ name, description, content, enabled, required_tool_names })
    router.push('/skills')
  }

  return (
    <div className="p-4 sm:p-6 max-w-3xl">
      <div className="flex flex-wrap items-center gap-3 justify-between mb-5">
        <div className="flex items-center gap-2 text-[12px] text-gray-400">
          <span onClick={() => router.push('/skills')} className="hover:text-gray-600 cursor-pointer">Skills</span>
          <ChevronRight className="w-3 h-3" />
          <span className="text-gray-700 font-medium">New skill</span>
        </div>
        <div className="flex gap-2">
          <button onClick={() => router.push('/skills')} className="inline-flex items-center gap-1.5 px-3 py-1.5 border border-gray-200 text-gray-600 text-[12px] rounded-lg hover:bg-gray-50">
            <X className="w-3.5 h-3.5" /> Discard
          </button>
          <button onClick={save} className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-purple-600 text-white text-[12px] rounded-lg hover:bg-purple-700">
            <Save className="w-3.5 h-3.5" /> Save Skill
          </button>
        </div>
      </div>
      {error && <div className="mb-4 text-sm text-red-600 bg-red-50 border border-red-200 rounded-lg px-3 py-2">{error}</div>}
      <div className="space-y-4">
        <Field label="Name"><input value={name} onChange={(e) => setName(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg" /></Field>
        <Field label="Description"><input value={description} onChange={(e) => setDescription(e.target.value)} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg" /></Field>
        <Field label="Instructions"><textarea value={content} onChange={(e) => setContent(e.target.value)} rows={12} className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg font-mono" /></Field>
        <Field label="Required tools">
          <input value={requiredTools} onChange={(e) => setRequiredTools(e.target.value)} placeholder="e.g. whatsapp_send_message, whatsapp_search_contacts" className="w-full text-[13px] px-3 py-2 border border-gray-200 rounded-lg" />
          <p className="text-[11px] text-gray-400 mt-1">Comma-separated tool names. With lazy tool loading, these tools are auto-activated when this skill is requested.</p>
        </Field>
        <label className="flex items-center gap-2 text-sm text-gray-700">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          Enabled
        </label>
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <label className="block"><span className="block text-[12px] font-medium text-gray-700 mb-1.5">{label}</span>{children}</label>
}
