'use client'

import { create } from 'zustand'

export type Toast = { id: number; message: string; variant: 'error' | 'success' }

interface ToastState {
  toasts: Toast[]
  push: (message: string, variant?: Toast['variant']) => void
  dismiss: (id: number) => void
}

let nextId = 0

export const useToastStore = create<ToastState>()((set) => ({
  toasts: [],
  push: (message, variant = 'error') => {
    const id = ++nextId
    set((s) => ({ toasts: [...s.toasts, { id, message, variant }] }))
    setTimeout(() => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })), 6000)
  },
  dismiss: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
}))

// A bare callable (not a hook) so this is reachable from the QueryClient's
// MutationCache config in providers.tsx, which runs outside any component —
// React Context wouldn't work there.
export const toast = (message: string, variant?: Toast['variant']) =>
  useToastStore.getState().push(message, variant)
