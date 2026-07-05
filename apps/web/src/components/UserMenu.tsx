'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useTheme } from 'next-themes'
import { Moon, Sun } from 'lucide-react'
import { useAuthStore } from '@/store/auth'
import { authAPI } from '@/lib/api'

export function UserMenu() {
  const { user, clearAuth } = useAuthStore()
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const { resolvedTheme, setTheme } = useTheme()
  const [mounted, setMounted] = useState(false)

  // Avoid rendering theme-dependent UI until mounted (resolvedTheme is
  // undefined on the server / first client render, which would otherwise
  // cause a hydration mismatch).
  useEffect(() => setMounted(true), [])

  const initials = user?.full_name
    ? user.full_name.split(' ').map((n: string) => n[0]).join('').toUpperCase().slice(0, 2)
    : '?'

  async function handleLogout() {
    try {
      await authAPI.logout()
    } catch { /* ignore */ }
    clearAuth()
    router.push('/login')
  }

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="w-9 h-9 rounded-full bg-accent text-white border-none cursor-pointer text-[13px] font-semibold flex items-center justify-center hover:bg-accent-hover transition-colors"
      >
        {initials}
      </button>

      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="absolute right-0 top-11 z-50 bg-background border border-border rounded-lg p-2 min-w-[200px] shadow-lg">
            <div className="px-3 py-2 border-b border-border mb-1">
              <p className="text-foreground m-0 text-sm font-medium truncate">{user?.full_name}</p>
              <p className="text-muted-foreground m-0 text-xs truncate">{user?.email}</p>
            </div>
            {mounted && (
              <button
                onClick={() => setTheme(resolvedTheme === 'dark' ? 'light' : 'dark')}
                className="flex items-center gap-2 w-full px-3 py-2 text-left bg-transparent border-none text-foreground text-sm rounded cursor-pointer hover:bg-muted"
              >
                {resolvedTheme === 'dark' ? <Sun className="w-3.5 h-3.5" /> : <Moon className="w-3.5 h-3.5" />}
                {resolvedTheme === 'dark' ? 'Light mode' : 'Dark mode'}
              </button>
            )}
            <button
              onClick={handleLogout}
              className="block w-full px-3 py-2 text-left bg-transparent border-none text-red-500 text-sm rounded cursor-pointer hover:bg-muted"
            >
              Sign out
            </button>
          </div>
        </>
      )}
    </div>
  )
}
