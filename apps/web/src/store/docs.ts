'use client'

import { create } from 'zustand'
import type { APIToken } from '@/types'

interface DocsState {
  selectedToken: APIToken | null
  selectedTokenRaw: string | null // the actual token value (stored per-session only)
  setSelectedToken: (token: APIToken | null, raw?: string) => void
}

export const useDocsStore = create<DocsState>()((set) => ({
  selectedToken: null,
  selectedTokenRaw: null,
  setSelectedToken: (token, raw) =>
    set({ selectedToken: token, selectedTokenRaw: raw ?? null }),
}))
