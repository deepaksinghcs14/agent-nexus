'use client'

import { useEffect, useState } from 'react'
import { usePathname } from 'next/navigation'
import { Menu, X } from 'lucide-react'
import { Sidebar } from '@/components/layout/Sidebar'
import { DemoBanner } from '@/components/layout/DemoBanner'
import { UserMenu } from '@/components/UserMenu'
import { InstallPrompt } from '@/components/layout/InstallPrompt'
import { DemoModeContext } from '@/context/demo-mode'
import { configAPI } from '@/lib/api'

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const [demoMode, setDemoMode] = useState(false)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)

  useEffect(() => {
    configAPI.get().then((c) => setDemoMode(c.demo_mode)).catch(() => {})
  }, [])

  // Close mobile nav on route change
  useEffect(() => { setMobileNavOpen(false) }, [pathname])

  if (pathname.startsWith('/login') || pathname.startsWith('/docs')) {
    return <>{children}</>
  }

  const isAdmin = pathname.startsWith('/admin')

  return (
    <DemoModeContext.Provider value={demoMode}>
      <div className="flex h-[100dvh] bg-white overflow-hidden flex-col">
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
            <header className="h-12 border-b border-gray-100 flex items-center justify-between px-4 bg-white flex-shrink-0">
              {/* Hamburger — mobile only */}
              <button
                className="md:hidden p-1.5 -ml-1 rounded-md text-gray-500 hover:text-gray-800 hover:bg-gray-100 transition-colors"
                onClick={() => setMobileNavOpen(true)}
                aria-label="Open navigation"
              >
                <Menu className="w-5 h-5" />
              </button>

              <span className="text-[13px] font-medium text-gray-800 md:block hidden">
                {isAdmin ? 'Administration' : 'Agent Nexus'}
              </span>
              <span className="text-[13px] font-medium text-gray-800 md:hidden">
                {isAdmin ? 'Administration' : 'Agent Nexus'}
              </span>

              <UserMenu />
            </header>
            <main className="flex-1 overflow-auto bg-white">
              {children}
            </main>
          </div>
        </div>
      </div>
      <InstallPrompt />
    </DemoModeContext.Provider>
  )
}
