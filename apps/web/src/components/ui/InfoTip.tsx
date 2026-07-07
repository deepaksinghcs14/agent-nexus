'use client'

import { useState } from 'react'
import { HelpCircle } from 'lucide-react'

/**
 * InfoTip — a small "?" icon that reveals an explanation on hover or tap. Used to
 * explain form fields inline so users don't have to guess what each setting does.
 */
export function InfoTip({ text }: { text: string }) {
  const [open, setOpen] = useState(false)
  if (!text) return null
  return (
    <span
      className="relative inline-flex items-center align-middle"
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
    >
      <button
        type="button"
        onClick={(e) => { e.preventDefault(); setOpen((v) => !v) }}
        className="text-gray-300 dark:text-gray-600 hover:text-purple-500 dark:hover:text-purple-400"
        aria-label="More info"
      >
        <HelpCircle className="w-3.5 h-3.5" />
      </button>
      {open && (
        <span
          role="tooltip"
          className="absolute left-1/2 bottom-full z-30 mb-1.5 -translate-x-1/2 w-60 rounded-lg bg-gray-900 dark:bg-gray-800 px-3 py-2 text-[11px] leading-relaxed text-gray-100 shadow-lg border border-gray-700"
        >
          {text}
        </span>
      )}
    </span>
  )
}
