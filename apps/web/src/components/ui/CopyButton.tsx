'use client'

import { useState } from 'react'
import { Check, Copy } from 'lucide-react'

export function CopyButton({ text, title = 'Copy' }: { text: string; title?: string }) {
  const [copied, setCopied] = useState(false)
  const handle = () => {
    navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }
  return (
    <button onClick={handle} className="p-1 rounded hover:bg-muted text-faint hover:text-foreground transition-colors" title={title}>
      {copied ? <Check className="w-3.5 h-3.5 text-good" /> : <Copy className="w-3.5 h-3.5" />}
    </button>
  )
}
