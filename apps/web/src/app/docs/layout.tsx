import type { Metadata } from 'next'
import Link from 'next/link'
import { Hexagon, BookOpen } from 'lucide-react'
import DocsTokenSelector from '@/components/docs/DocsTokenSelector'

export const metadata: Metadata = { title: 'Docs — Agent Nexus' }

const NAV = [
  {
    group: 'Getting Started',
    items: [
      { label: 'Overview',          href: '/docs' },
      { label: 'What is an Agent',  href: '/docs/what-is-an-agent' },
      { label: 'Workflows',          href: '/docs/what-is-an-agent-group' },
    ],
  },
  {
    group: 'Platform Concepts',
    items: [
      { label: 'Tools',      href: '/docs/what-is-a-tool' },
      { label: 'MCP Servers', href: '/docs/mcp-servers' },
      { label: 'Connectors', href: '/docs/what-is-a-connector' },
      { label: 'Skills',     href: '/docs/skills' },
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
  return (
    <div className="flex min-h-screen" style={{ fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, sans-serif' }}>

      {/* Left nav — dark, matching app sidebar */}
      <aside
        className="w-60 shrink-0 flex flex-col sticky top-0 h-screen overflow-y-auto"
        style={{ background: '#13111f', borderRight: '1px solid rgba(255,255,255,0.06)' }}
      >
        {/* Logo */}
        <Link
          href="/dashboard"
          className="flex items-center gap-2.5 px-5 py-4 border-b"
          style={{ borderColor: 'rgba(255,255,255,0.06)' }}
        >
          <div
            className="w-6 h-6 rounded-md flex items-center justify-center"
            style={{ background: '#534AB7' }}
          >
            <Hexagon className="w-3.5 h-3.5 text-white" />
          </div>
          <span className="text-sm font-semibold text-white">Agent Nexus</span>
        </Link>

        {/* Docs label */}
        <div className="px-5 pt-5 pb-2 flex items-center gap-2">
          <BookOpen className="w-3.5 h-3.5" style={{ color: '#534AB7' }} />
          <span className="text-xs font-semibold uppercase tracking-widest" style={{ color: '#534AB7' }}>
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
                <DocsNavLink key={item.href} label={item.label} href={item.href} />
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
          >
            ← Back to app
          </Link>
        </div>
      </aside>

      {/* Right panel */}
      <div className="flex-1 flex flex-col min-w-0 bg-white">

        {/* Token bar */}
        <div
          className="sticky top-0 z-10 flex items-center gap-4 px-8 py-2.5 border-b"
          style={{ background: '#13111f', borderColor: 'rgba(255,255,255,0.06)' }}
        >
          <span className="text-xs shrink-0" style={{ color: 'rgba(255,255,255,0.35)' }}>
            Test with:
          </span>
          <DocsTokenSelector />
        </div>

        {/* Content */}
        <main className="flex-1 px-14 py-12 max-w-3xl w-full">
          {children}
        </main>
      </div>
    </div>
  )
}

// Server-side nav link — active state handled client-side via a wrapper
function DocsNavLink({ label, href }: { label: string; href: string }) {
  return (
    <Link
      href={href}
      className="flex items-center gap-2 px-2 py-1.5 rounded-md text-sm transition-colors mb-0.5"
      style={{ color: 'rgba(255,255,255,0.55)' }}
    >
      {label}
    </Link>
  )
}
