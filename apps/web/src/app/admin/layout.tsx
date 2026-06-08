import type { Metadata } from 'next'

export const metadata: Metadata = {
  title: 'Agent Nexus Admin',
}

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return children
}
