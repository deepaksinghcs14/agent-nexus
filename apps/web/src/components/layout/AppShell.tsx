'use client'

import { useEffect, useState } from 'react'
import { usePathname } from 'next/navigation'
import { Sidebar } from '@/components/layout/Sidebar'
import { DemoBanner } from '@/components/layout/DemoBanner'
import { UserMenu } from '@/components/UserMenu'
import { DemoModeContext } from '@/context/demo-mode'
import { configAPI } from '@/lib/api'

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const [demoMode, setDemoMode] = useState(false)

  useEffect(() => {
    configAPI.get().then((c) => setDemoMode(c.demo_mode)).catch(() => {})
  }, [])

  if (pathname.startsWith('/login') || pathname.startsWith('/docs')) {
    return <>{children}</>
  }

  const isAdmin = pathname.startsWith('/admin')

  return (
    <DemoModeContext.Provider value={demoMode}>
      <div className="flex h-screen bg-white overflow-hidden flex-col">
        {demoMode && <DemoBanner />}
        <div className="flex flex-1 overflow-hidden min-h-0">
          <Sidebar isAdmin={isAdmin} />
          <div className="flex-1 flex flex-col overflow-hidden min-w-0">
            <header className="h-11 border-b border-gray-100 flex items-center justify-between px-5 bg-white flex-shrink-0">
              <span className="text-[13px] font-medium text-gray-800">
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
    </DemoModeContext.Provider>
  )
}
