'use client'

import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Search } from 'lucide-react'
import { toolsAPI } from '@/lib/api'
import type { Tool } from '@/types'
import { toolCategory } from '@/lib/tool-category'

/**
 * ToolPicker — a searchable, category-grouped multi-select of workspace tools.
 * Selection is by tool NAME (skills store required_tool_names as names). Used on
 * the skill create/edit pages so users pick tools instead of typing their names.
 */
export function ToolPicker({
  selected,
  onChange,
}: {
  selected: string[]
  onChange: (names: string[]) => void
}) {
  const [search, setSearch] = useState('')
  const { data, isLoading } = useQuery({
    queryKey: ['tools'],
    queryFn: () => toolsAPI.list() as Promise<{ data: Tool[] }>,
  })
  const tools = data?.data ?? []
  const selectedSet = useMemo(() => new Set(selected), [selected])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return tools
    return tools.filter(
      (t) =>
        t.name.toLowerCase().includes(q) ||
        (t.description ?? '').toLowerCase().includes(q) ||
        toolCategory(t).toLowerCase().includes(q),
    )
  }, [tools, search])

  const grouped = useMemo(() => {
    const map = new Map<string, Tool[]>()
    for (const t of filtered) {
      const cat = toolCategory(t)
      if (!map.has(cat)) map.set(cat, [])
      map.get(cat)!.push(t)
    }
    return [...map.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  }, [filtered])

  const toggle = (name: string) => {
    if (selectedSet.has(name)) onChange(selected.filter((n) => n !== name))
    else onChange([...selected, name])
  }

  return (
    <div className="border border-gray-200 dark:border-gray-700 rounded-lg overflow-hidden">
      <div className="flex items-center gap-2 px-3 py-2 border-b border-gray-100 dark:border-gray-800">
        <Search className="w-3.5 h-3.5 text-gray-400" />
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search tools by name, description, or category…"
          className="flex-1 text-[13px] bg-transparent outline-none"
        />
        {selected.length > 0 && (
          <span className="text-[11px] text-gray-500 dark:text-gray-400">{selected.length} selected</span>
        )}
      </div>
      <div className="max-h-72 overflow-y-auto p-1">
        {isLoading && <p className="text-[12px] text-gray-400 px-2 py-3">Loading tools…</p>}
        {!isLoading && grouped.length === 0 && (
          <p className="text-[12px] text-gray-400 px-2 py-3">No tools match “{search}”.</p>
        )}
        {grouped.map(([cat, catTools]) => (
          <div key={cat} className="mb-1">
            <div className="px-2 py-1 text-[10px] font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">
              {cat}
            </div>
            {catTools.map((t) => (
              <label
                key={t.id}
                className="flex items-start gap-2 px-2 py-1.5 rounded-md hover:bg-gray-50 dark:hover:bg-gray-800 cursor-pointer"
              >
                <input
                  type="checkbox"
                  checked={selectedSet.has(t.name)}
                  onChange={() => toggle(t.name)}
                  className="mt-0.5"
                />
                <span className="min-w-0">
                  <span className="block text-[13px] text-gray-800 dark:text-gray-200 font-mono truncate">{t.name}</span>
                  {t.description && (
                    <span className="block text-[11px] text-gray-400 dark:text-gray-500 truncate">{t.description}</span>
                  )}
                </span>
              </label>
            ))}
          </div>
        ))}
      </div>
      {selected.length > 0 && (
        <div className="flex flex-wrap gap-1 px-3 py-2 border-t border-gray-100 dark:border-gray-800">
          {selected.map((name) => (
            <button
              key={name}
              onClick={() => toggle(name)}
              className="inline-flex items-center gap-1 px-2 py-0.5 text-[11px] font-mono bg-purple-50 dark:bg-purple-500/10 text-purple-700 dark:text-purple-300 rounded-md"
            >
              {name} ✕
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
