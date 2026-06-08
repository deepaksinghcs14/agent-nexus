import type { Metadata } from 'next'
import './globals.css'
import 'highlight.js/styles/github-dark.css'
import { Providers } from '@/providers'
import { AppShell } from '@/components/layout/AppShell'

export const metadata: Metadata = {
  title: 'Agent Nexus',
  description: 'Self-hosted model-agnostic AI agent orchestration platform',
  icons: {
    icon: '/icon.svg',
  },
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body style={{ fontFamily: 'Inter, system-ui, sans-serif' }}>
        <Providers>
          <AppShell>{children}</AppShell>
        </Providers>
      </body>
    </html>
  )
}
