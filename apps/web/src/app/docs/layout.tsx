'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useState } from 'react'
import { BookOpen, Menu } from 'lucide-react'
import { cn } from '@/lib/utils'
import DocsTokenSelector from '@/components/docs/DocsTokenSelector'

const NAV = [
  {
    group: 'Getting Started',
    items: [
      { label: 'Overview',          href: '/docs' },
      { label: 'What is an Agent',  href: '/docs/what-is-an-agent' },
      { label: 'What is a Workflow', href: '/docs/what-is-an-agent-group' },
      { label: 'Workflow Node Types', href: '/docs/workflows' },
    ],
  },
  {
    group: 'Platform Concepts',
    items: [
      { label: 'Tools',      href: '/docs/what-is-a-tool' },
      { label: 'MCP Servers', href: '/docs/mcp-servers' },
      { label: 'Connectors', href: '/docs/what-is-a-connector' },
      { label: 'Skills',     href: '/docs/skills' },
      { label: 'Evals',      href: '/docs/evals' },
      { label: 'Claude Code Pipeline', href: '/docs/claude-code-pipeline' },
    ],
  },
  {
    group: 'Messaging Channels',
    items: [
      { label: 'Nexus Gateway', href: '/docs/gateway' },
    ],
  },
  {
    group: 'API Integration',
    items: [
      { label: 'API Tokens',         href: '/docs/api-tokens' },
      { label: 'Invoke API',         href: '/docs/invoke-api' },
      { label: 'Webhook Triggers',   href: '/docs/webhook-triggers' },
      { label: 'Run States',         href: '/docs/run-states' },
      { label: 'SSE Events',         href: '/docs/sse-events' },
    ],
  },
  {
    group: 'Reference',
    items: [
      { label: 'Agent Config',       href: '/docs/agent-configuration' },
      { label: 'Architecture',       href: '/architecture' },
    ],
  },
]

export default function DocsLayout({ children }: { children: React.ReactNode }) {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const pathname = usePathname()

  return (
    <div className="flex min-h-screen bg-background font-sans">

      {/* Mobile overlay */}
      {sidebarOpen && (
        <div className="fixed inset-0 z-20 bg-black/50 lg:hidden" onClick={() => setSidebarOpen(false)} />
      )}

      {/* Left nav — matches the app shell */}
      <aside
        className={cn(
          'fixed inset-y-0 left-0 z-30 w-60 flex flex-col h-screen overflow-y-auto bg-surface border-r border-border transition-transform duration-200 lg:sticky lg:top-0 lg:translate-x-0',
          sidebarOpen ? 'translate-x-0' : '-translate-x-full'
        )}
      >
        {/* Logo */}
        <Link
          href="/dashboard"
          className="flex items-center gap-2.5 px-5 py-4 border-b border-border hover:opacity-80 transition-opacity"
          onClick={() => setSidebarOpen(false)}
        >
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/icon.svg" alt="Agent Nexus" className="w-7 h-7 rounded-lg shrink-0 shadow-sm" />
          <span className="text-sm font-semibold tracking-tight text-foreground">Agent Nexus</span>
        </Link>

        {/* Docs label */}
        <div className="px-5 pt-5 pb-1 flex items-center gap-2">
          <BookOpen className="w-3.5 h-3.5 shrink-0 text-accent dark:text-accent-bright" />
          <span className="eyebrow">Documentation</span>
        </div>

        {/* Nav groups */}
        <nav className="flex-1 px-3 py-2 pb-6">
          {NAV.map((section) => (
            <div key={section.group} className="mb-4">
              <p className="px-2 pt-2 pb-1 eyebrow">{section.group}</p>
              {section.items.map((item) => {
                const active = pathname === item.href
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    onClick={() => setSidebarOpen(false)}
                    className={cn(
                      'relative flex items-center gap-2 mx-1 px-2.5 py-1.5 rounded-lg text-[13px] transition-colors mb-0.5',
                      active
                        ? 'text-accent dark:text-accent-bright bg-accent/[0.09] dark:bg-accent-bright/[0.12] font-medium'
                        : 'text-muted-foreground hover:text-foreground hover:bg-muted'
                    )}
                  >
                    {active && <span className="absolute -left-1 top-1.5 bottom-1.5 w-[2.5px] rounded-full bg-accent dark:bg-accent-bright" />}
                    {item.label}
                  </Link>
                )
              })}
            </div>
          ))}
        </nav>

        {/* Back to app */}
        <div className="px-3 pb-4 border-t border-border pt-3">
          <Link
            href="/dashboard"
            className="flex items-center gap-2 px-3 py-2 rounded-lg text-xs text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
            onClick={() => setSidebarOpen(false)}
          >
            ← Back to app
          </Link>
        </div>
      </aside>

      {/* Right panel */}
      <div className="flex-1 flex flex-col min-w-0 bg-background">

        {/* Token bar */}
        <div className="sticky top-0 z-10 flex items-center gap-3 px-4 sm:px-8 py-2.5 border-b border-border bg-surface/80 backdrop-blur">
          {/* Mobile hamburger */}
          <button
            className="lg:hidden p-1 rounded text-muted-foreground hover:text-foreground shrink-0"
            onClick={() => setSidebarOpen(true)}
            aria-label="Open navigation"
          >
            <Menu className="w-5 h-5" />
          </button>

          <span className="text-xs shrink-0 hidden sm:block text-muted-foreground">Test with:</span>
          <div className="min-w-0 flex-1 sm:flex-none">
            <DocsTokenSelector />
          </div>
        </div>

        {/* Content */}
        <main className="flex-1 px-4 sm:px-8 lg:px-14 py-8 lg:py-12 w-full">
          {children}
        </main>
      </div>
    </div>
  )
}
