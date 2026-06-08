'use client'

import { create } from 'zustand'
import type { User, Workspace, WorkspaceWithRole } from '@/types'

interface AuthState {
  user: User | null
  workspace: Workspace | null
  workspaceId: string | null
  workspaces: WorkspaceWithRole[]
  accessToken: string | null
  isAuthenticated: boolean
  isInitialized: boolean

  setAuth: (data: { user: User; workspaceId: string; accessToken: string; workspace?: Workspace }) => void
  setWorkspace: (ws: Workspace) => void
  setWorkspaces: (ws: WorkspaceWithRole[]) => void
  switchWorkspace: (workspaceId: string, accessToken: string) => void
  clearAuth: () => void
  setInitialized: () => void
}

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  workspace: null,
  workspaceId: null,
  workspaces: [],
  accessToken: null,
  isAuthenticated: false,
  isInitialized: false,

  setAuth: ({ user, workspaceId, accessToken, workspace }) => {
    if (typeof window !== 'undefined') {
      localStorage.setItem('access_token', accessToken)
    }
    set({ user, workspaceId, accessToken, workspace: workspace ?? null, isAuthenticated: true })
  },

  setWorkspace: (ws) => set({ workspace: ws }),

  setWorkspaces: (ws) => set({ workspaces: ws }),

  switchWorkspace: (workspaceId, accessToken) => {
    if (typeof window !== 'undefined') {
      localStorage.setItem('access_token', accessToken)
    }
    const matched = get().workspaces.find((w) => w.id === workspaceId) ?? null
    set({ workspaceId, accessToken, workspace: matched })
  },

  clearAuth: () => {
    if (typeof window !== 'undefined') {
      localStorage.removeItem('access_token')
    }
    set({ user: null, workspace: null, workspaceId: null, workspaces: [], accessToken: null, isAuthenticated: false })
  },

  setInitialized: () => set({ isInitialized: true }),
}))
