'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import {
  Bot, GitBranch, Wrench, Plug, Link2,
  MessageSquare, History,
  Activity, GitCommitHorizontal, Brain, BarChart2,
  Key, Settings, BookOpen,
  LayoutDashboard, Users, Building2, Shield, ClipboardList,
  Hexagon, ShieldCheck,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/store/auth'
import { WorkspaceSwitcher } from './WorkspaceSwitcher'

type NavItem = { label: string; href: string; icon: React.ElementType }
type NavGroup = { label: string; items: NavItem[] }

const userNav: NavGroup[] = [
  {
    label: 'Build',
    items: [
      { label: 'Agents',       href: '/agents',       icon: Bot },
      { label: 'Workflows',    href: '/workflows',    icon: GitBranch },
      { label: 'Tools',        href: '/tools',        icon: Wrench },
      { label: 'MCP Servers',  href: '/mcp-servers',  icon: Plug },
      { label: 'Connectors',   href: '/connectors',   icon: Link2 },
    ],
  },
  {
    label: 'Run',
    items: [
      { label: 'Playground',    href: '/playground',    icon: MessageSquare },
      { label: 'Conversations', href: '/conversations', icon: History },
    ],
  },
  {
    label: 'Observe',
    items: [
      { label: 'Runs',   href: '/runs',   icon: Activity },
      { label: 'Traces', href: '/traces', icon: GitCommitHorizontal },
      { label: 'Memory', href: '/memory', icon: Brain },
      { label: 'Usage',  href: '/usage',  icon: BarChart2 },
    ],
  },
  {
    label: 'Settings',
    items: [
      { label: 'Providers',   href: '/settings/providers',   icon: Key },
      { label: 'API Tokens',  href: '/settings/api-tokens',  icon: Key },
      { label: 'Workspace',   href: '/settings/workspace',   icon: Settings },
    ],
  },
  {
    label: 'Developer',
    items: [
      { label: 'Docs', href: '/docs', icon: BookOpen },
    ],
  },
]

const adminNav: NavGroup[] = [
  {
    label: 'Admin',
    items: [
      { label: 'Overview',    href: '/admin/overview',    icon: LayoutDashboard },
      { label: 'Users',       href: '/admin/users',       icon: Users },
      { label: 'Workspaces',  href: '/admin/workspaces',  icon: Building2 },
      { label: 'Policies',    href: '/admin/policies',    icon: Shield },
      { label: 'Audit Logs',  href: '/admin/audit-logs',  icon: ClipboardList },
    ],
  },
]

function NavGroup({ group, pathname }: { group: NavGroup; pathname: string }) {
  return (
    <div className="mb-1">
      <p className="px-4 pt-3 pb-1 text-[10px] font-medium text-white/30 uppercase tracking-wider">
        {group.label}
      </p>
      {group.items.map((item) => {
        const active = pathname === item.href || pathname.startsWith(item.href + '/')
        return (
          <Link
            key={item.href + item.label}
            href={item.href}
            className={cn(
              'flex items-center gap-2 px-4 py-1.5 text-[13px] transition-colors',
              active
                ? 'text-accent-muted bg-white/[0.06] font-medium'
                : 'text-white/50 hover:text-white/80 hover:bg-white/[0.04]'
            )}
          >
            <item.icon className="w-3.5 h-3.5 flex-shrink-0" />
            {item.label}
          </Link>
        )
      })}
    </div>
  )
}

export function Sidebar({ isAdmin = false }: { isAdmin?: boolean }) {
  const pathname = usePathname()
  const user = useAuthStore((s) => s.user)
  const userIsAdmin = user?.is_admin ?? false
  const groups = isAdmin ? adminNav : userNav

  return (
    <aside className="w-48 min-w-[192px] flex flex-col h-full bg-sidebar border-r border-white/[0.06]">
      {/* Logo */}
      <Link
        href="/dashboard"
        className="flex items-center gap-2 px-4 py-3 border-b border-white/[0.06] transition-colors hover:bg-white/[0.04]"
        aria-label="Open dashboard"
      >
        <div className="w-6 h-6 bg-accent rounded-md flex items-center justify-center">
          <Hexagon className="w-3.5 h-3.5 text-white" />
        </div>
        <span className="text-sm font-medium text-white">Agent Nexus</span>
        {isAdmin && (
          <span className="ml-auto text-[9px] bg-accent/30 text-accent-muted px-1.5 py-0.5 rounded font-medium">
            ADMIN
          </span>
        )}
      </Link>

      {/* Workspace switcher */}
      {!isAdmin && <WorkspaceSwitcher />}

      {/* Nav */}
      <nav className="flex-1 overflow-y-auto py-2">
        {groups.map((group) => (
          <NavGroup key={group.label} group={group} pathname={pathname} />
        ))}
      </nav>

      {/* Admin link at the bottom — only for admin users in the regular (non-admin) sidebar */}
      {!isAdmin && userIsAdmin && (
        <div className="border-t border-white/[0.06] p-2">
          <Link
            href="/admin/overview"
            className={cn(
              'flex items-center gap-2 px-3 py-2 rounded-md text-[12px] transition-colors w-full',
              pathname.startsWith('/admin')
                ? 'text-amber-300 bg-white/[0.06] font-medium'
                : 'text-white/40 hover:text-amber-300 hover:bg-white/[0.04]'
            )}
          >
            <ShieldCheck className="w-3.5 h-3.5 flex-shrink-0" />
            Admin dashboard
          </Link>
        </div>
      )}

      {/* Back to app link when in admin sidebar */}
      {isAdmin && (
        <div className="border-t border-white/[0.06] p-2">
          <Link
            href="/dashboard"
            className="flex items-center gap-2 px-3 py-2 rounded-md text-[12px] text-white/40 hover:text-white/70 hover:bg-white/[0.04] transition-colors w-full"
          >
            <Hexagon className="w-3.5 h-3.5 flex-shrink-0" />
            Back to app
          </Link>
        </div>
      )}
    </aside>
  )
}
