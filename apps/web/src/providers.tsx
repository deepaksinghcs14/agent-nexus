'use client'

import { useState, useEffect, useRef } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ThemeProvider } from 'next-themes'
import { useAuthStore } from '@/store/auth'
import { authAPI } from '@/lib/api'
import type { User, Workspace, WorkspaceWithRole } from '@/types'

function AuthInitializer() {
  const { setAuth, setWorkspaces, clearAuth, setInitialized, isInitialized } = useAuthStore()
  const ran = useRef(false)

  useEffect(() => {
    if (ran.current) return
    ran.current = true

    const token = localStorage.getItem('access_token')
    if (!token) {
      setInitialized()
      return
    }

    const applyMe = (data: Record<string, unknown>, tok: string) => {
      const ws = data.workspace as unknown as Workspace
      setAuth({ user: data.user as unknown as User, workspaceId: ws?.id ?? '', accessToken: tok, workspace: ws })
      setWorkspaces((data.workspaces as unknown as WorkspaceWithRole[]) ?? [])
    }

    authAPI.me()
      .then((data) => applyMe(data as unknown as Record<string, unknown>, token))
      .catch(() => {
        // Try refresh
        authAPI.refresh()
          .then(({ access_token }) => {
            localStorage.setItem('access_token', access_token)
            return authAPI.me()
          })
          .then((data) => {
            const token2 = localStorage.getItem('access_token') ?? ''
            applyMe(data as unknown as Record<string, unknown>, token2)
          })
          .catch(() => {
            clearAuth()
          })
      })
      .finally(() => {
        setInitialized()
      })
  }, [setAuth, setWorkspaces, clearAuth, setInitialized])

  return null
}

export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { staleTime: 0, retry: 1 },
        },
      })
  )

  return (
    <ThemeProvider attribute="class" defaultTheme="light" enableSystem={false}>
      <QueryClientProvider client={queryClient}>
        <AuthInitializer />
        {children}
      </QueryClientProvider>
    </ThemeProvider>
  )
}
