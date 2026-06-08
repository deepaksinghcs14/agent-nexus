'use client'

import { usePathname } from 'next/navigation'
import { Sidebar } from '@/components/layout/Sidebar'
import { UserMenu } from '@/components/UserMenu'

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()

  if (pathname.startsWith('/login') || pathname.startsWith('/docs')) {
    return <>{children}</>
  }

  const isAdmin = pathname.startsWith('/admin')

  return (
    <div className="flex h-screen bg-white overflow-hidden">
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
  )
}
