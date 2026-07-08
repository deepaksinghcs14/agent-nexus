'use client'

import { useState, useEffect } from 'react'
import { Key, X, CheckCircle2 } from 'lucide-react'
import { apiTokensAPI } from '@/lib/api'
import { useDocsStore } from '@/store/docs'
import type { APIToken } from '@/types'

export default function DocsTokenSelector() {
  const [tokens, setTokens] = useState<APIToken[]>([])
  const [value, setValue] = useState('')
  const { selectedTokenRaw, setSelectedToken } = useDocsStore()

  useEffect(() => {
    apiTokensAPI.list().then((res) => setTokens(res.data ?? [])).catch(() => {})
  }, [])

  const handleChange = (raw: string) => {
    setValue(raw)
    if (raw.startsWith('anx_') && raw.length > 12) {
      // Try to match against known tokens by prefix
      const matched = tokens.find((t) => raw.startsWith(t.token_prefix)) ?? null
      setSelectedToken(matched, raw)
    } else {
      setSelectedToken(null, undefined)
    }
  }

  const handleClear = () => {
    setValue('')
    setSelectedToken(null, undefined)
  }

  const matched = selectedTokenRaw
    ? (tokens.find((t) => selectedTokenRaw.startsWith(t.token_prefix)) ?? null)
    : null

  return (
    <div className="flex items-center gap-2 sm:gap-3 min-w-0">
      <Key className="w-3.5 h-3.5 shrink-0 text-faint" />

      <div className="relative flex items-center min-w-0 flex-1 sm:flex-none">
        <input
          type="text"
          value={value}
          onChange={(e) => handleChange(e.target.value)}
          placeholder="Paste API token (anx_...)"
          className={`w-full sm:w-52 md:w-64 pl-3 pr-7 py-1 text-xs font-mono rounded border bg-surface-2 text-foreground placeholder-faint focus:outline-none focus:ring-1 focus:ring-accent ${selectedTokenRaw ? 'border-accent' : 'border-border-strong'}`}
        />
        {value && (
          <button onClick={handleClear} className="absolute right-2 text-faint hover:text-foreground">
            <X className="w-3 h-3" />
          </button>
        )}
      </div>

      {selectedTokenRaw && (
        <span className="hidden sm:flex items-center gap-1.5 text-xs text-good shrink-0">
          <CheckCircle2 className="w-3.5 h-3.5" />
          {matched ? matched.name : 'Ready'}
        </span>
      )}

      {!selectedTokenRaw && (
        <a href="/settings/api-tokens" className="hidden sm:block text-xs shrink-0 text-muted-foreground hover:text-foreground">
          Get a token →
        </a>
      )}
    </div>
  )
}
