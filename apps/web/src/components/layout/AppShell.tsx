'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { Menu, Sparkles } from 'lucide-react'
import { Sidebar } from '@/components/layout/Sidebar'
import { DemoBanner } from '@/components/layout/DemoBanner'
import { UserMenu } from '@/components/UserMenu'
import { InstallPrompt } from '@/components/layout/InstallPrompt'
import { DemoModeContext } from '@/context/demo-mode'
import { configAPI } from '@/lib/api'

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const router = useRouter()
  const [demoMode, setDemoMode] = useState(false)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)

  useEffect(() => {
    configAPI.get().then((c) => setDemoMode(c.demo_mode)).catch(() => {})
  }, [])

  // Close mobile nav on route change
  useEffect(() => { setMobileNavOpen(false) }, [pathname])

  // ⌘K / Ctrl+K → jump to Nexus AI (the command bar shortcut).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        router.push('/nexus-ai')
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [router])

  if (pathname.startsWith('/login') || pathname.startsWith('/docs')) {
    return <>{children}</>
  }

  const isAdmin = pathname.startsWith('/admin')

  return (
    <DemoModeContext.Provider value={demoMode}>
      <div className="flex h-[100dvh] bg-background overflow-hidden flex-col">
        {demoMode && <DemoBanner />}
        <div className="flex flex-1 overflow-hidden min-h-0">

          {/* Desktop sidebar — hidden on mobile */}
          <div className="hidden md:block flex-shrink-0">
            <Sidebar isAdmin={isAdmin} />
          </div>

          {/* Mobile drawer overlay */}
          {mobileNavOpen && (
            <div
              className="fixed inset-0 z-40 bg-black/50 md:hidden"
              onClick={() => setMobileNavOpen(false)}
            />
          )}

          {/* Mobile drawer */}
          <div className={`
            fixed inset-y-0 left-0 z-50 md:hidden flex-shrink-0
            transition-transform duration-200
            ${mobileNavOpen ? 'translate-x-0' : '-translate-x-full'}
          `}>
            <Sidebar isAdmin={isAdmin} onClose={() => setMobileNavOpen(false)} />
          </div>

          <div className="flex-1 flex flex-col overflow-hidden min-w-0">
            <header className="h-14 border-b border-border flex items-center gap-3 px-4 bg-surface/80 backdrop-blur flex-shrink-0 sticky top-0 z-10">
              {/* Hamburger — mobile only */}
              <button
                className="md:hidden p-1.5 -ml-1 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
                onClick={() => setMobileNavOpen(true)}
                aria-label="Open navigation"
              >
                <Menu className="w-5 h-5" />
              </button>

              {isAdmin && (
                <span className="text-[13px] font-medium text-foreground hidden md:block">Administration</span>
              )}

              {/* Command bar — jumps to Nexus AI (also ⌘K) */}
              <Link
                href="/nexus-ai"
                className="ml-auto md:ml-2 flex items-center gap-2.5 min-w-0 flex-1 max-w-md px-3 py-2 border border-border rounded-[10px] bg-surface-2 text-muted-foreground text-[13px] hover:border-border-strong transition-colors"
              >
                <Sparkles className="w-4 h-4 text-accent dark:text-accent-bright flex-shrink-0" />
                <span className="truncate">Ask Nexus AI…</span>
                <span className="ml-auto font-mono text-[10.5px] border border-border rounded px-1.5 py-0.5 text-faint bg-surface hidden sm:block">⌘K</span>
              </Link>

              <UserMenu />
            </header>
            <main className="flex-1 overflow-auto bg-background">
              {children}
            </main>
          </div>
        </div>
      </div>
      <InstallPrompt />
    </DemoModeContext.Provider>
  )
}
