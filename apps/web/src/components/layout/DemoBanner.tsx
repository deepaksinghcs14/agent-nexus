'use client'

import { ExternalLink } from 'lucide-react'

const GITHUB_URL = 'https://github.com/deepaksingh/agent-nexus'

export function DemoBanner() {
  return (
    <div className="w-full bg-amber-50 border-b border-amber-200 px-4 py-2 flex items-center justify-between flex-shrink-0">
      <p className="text-[12px] text-amber-800">
        <span className="font-semibold">Demo mode</span>
        {' — '}
        You&apos;re exploring Agent Nexus in a live demo. MCP servers, connectors, and API tokens are restricted.
      </p>
      <a
        href={GITHUB_URL}
        target="_blank"
        rel="noopener noreferrer"
        className="inline-flex items-center gap-1 text-[11px] font-medium text-amber-700 hover:text-amber-900 whitespace-nowrap ml-4"
      >
        Self-host for free
        <ExternalLink size={11} />
      </a>
    </div>
  )
}
