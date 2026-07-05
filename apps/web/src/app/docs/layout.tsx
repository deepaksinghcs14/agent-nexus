'use client'

import Link from 'next/link'
import { useState } from 'react'
import { Hexagon, BookOpen, Menu } from 'lucide-react'
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

  return (
    <div className="flex min-h-screen" style={{ fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, sans-serif' }}>

      {/* Mobile overlay */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-20 bg-black/50 lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Left nav — dark, matching app sidebar */}
      <aside
        className={`fixed inset-y-0 left-0 z-30 w-60 flex flex-col h-screen overflow-y-auto transition-transform duration-200 lg:sticky lg:top-0 lg:translate-x-0 ${sidebarOpen ? 'translate-x-0' : '-translate-x-full'}`}
        style={{ background: '#13111f', borderRight: '1px solid rgba(255,255,255,0.06)' }}
      >
        {/* Logo */}
        <Link
          href="/dashboard"
          className="flex items-center gap-2.5 px-5 py-4 border-b"
          style={{ borderColor: 'rgba(255,255,255,0.06)' }}
          onClick={() => setSidebarOpen(false)}
        >
          <div
            className="w-6 h-6 rounded-md flex items-center justify-center shrink-0"
            style={{ background: '#7c3aed' }}
          >
            <Hexagon className="w-3.5 h-3.5 text-white" />
          </div>
          <span className="text-sm font-semibold text-white">Agent Nexus</span>
        </Link>

        {/* Docs label */}
        <div className="px-5 pt-5 pb-2 flex items-center gap-2">
          <BookOpen className="w-3.5 h-3.5 shrink-0" style={{ color: '#7c3aed' }} />
          <span className="text-xs font-semibold uppercase tracking-widest" style={{ color: '#7c3aed' }}>
            Documentation
          </span>
        </div>

        {/* Nav groups */}
        <nav className="flex-1 px-3 pb-6">
          {NAV.map((section) => (
            <div key={section.group} className="mb-5">
              <p className="px-2 pb-1.5 text-[10px] font-semibold uppercase tracking-wider" style={{ color: 'rgba(255,255,255,0.3)' }}>
                {section.group}
              </p>
              {section.items.map((item) => (
                <Link
                  key={item.href}
                  href={item.href}
                  className="flex items-center gap-2 px-2 py-1.5 rounded-md text-sm transition-colors mb-0.5"
                  style={{ color: 'rgba(255,255,255,0.55)' }}
                  onClick={() => setSidebarOpen(false)}
                >
                  {item.label}
                </Link>
              ))}
            </div>
          ))}
        </nav>

        {/* Back to app */}
        <div className="px-3 pb-4 border-t pt-4" style={{ borderColor: 'rgba(255,255,255,0.06)' }}>
          <Link
            href="/dashboard"
            className="flex items-center gap-2 px-3 py-2 rounded-md text-xs transition-colors"
            style={{ color: 'rgba(255,255,255,0.35)' }}
            onClick={() => setSidebarOpen(false)}
          >
            ← Back to app
          </Link>
        </div>
      </aside>

      {/* Right panel */}
      <div className="flex-1 flex flex-col min-w-0 bg-white dark:bg-gray-900">

        {/* Token bar */}
        <div
          className="sticky top-0 z-10 flex items-center gap-3 px-4 sm:px-8 py-2.5 border-b"
          style={{ background: '#13111f', borderColor: 'rgba(255,255,255,0.06)' }}
        >
          {/* Mobile hamburger */}
          <button
            className="lg:hidden p-1 rounded text-white/50 hover:text-white/80 shrink-0"
            onClick={() => setSidebarOpen(true)}
            aria-label="Open navigation"
          >
            <Menu className="w-5 h-5" />
          </button>

          <span className="text-xs shrink-0 hidden sm:block" style={{ color: 'rgba(255,255,255,0.35)' }}>
            Test with:
          </span>
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
