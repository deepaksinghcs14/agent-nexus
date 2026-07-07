'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import {
  Bot, GitBranch, Wrench, Plug, Link2, Zap, Sparkles, Radio, BookMarked,
  MessageSquare, History,
  Activity, Brain, BarChart2, Timer, FlaskConical,
  Key, Settings, BookOpen,
  LayoutDashboard, Users, Building2, Shield, ClipboardList,
  ShieldCheck, PanelLeftClose, PanelLeftOpen, SquareTerminal, X,
  ChevronDown, ChevronRight,
} from 'lucide-react'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/store/auth'
import { WorkspaceSwitcher } from './WorkspaceSwitcher'

type NavItem = { label: string; href: string; icon: React.ElementType }
type NavGroup = { label: string; items: NavItem[] }

const userNav: NavGroup[] = [
  {
    label: 'Build',
    items: [
      { label: 'Nexus AI',      href: '/nexus-ai',     icon: Sparkles },
      { label: 'Agents',        href: '/agents',        icon: Bot },
      { label: 'Workflows',     href: '/workflows',     icon: GitBranch },
      { label: 'Claude Code',   href: '/claude-code',   icon: SquareTerminal },
    ],
  },
  {
    label: 'Integrations',
    items: [
      { label: 'Tools',         href: '/tools',         icon: Wrench },
      { label: 'MCP Servers',   href: '/mcp-servers',   icon: Plug },
      { label: 'Connectors',    href: '/connectors',    icon: Link2 },
      { label: 'Skills',        href: '/skills',        icon: BookMarked },
      { label: 'Gateway',       href: '/gateway',       icon: Radio },
      { label: 'Triggers',      href: '/triggers',      icon: Zap },
    ],
  },
  {
    label: 'Chat',
    items: [
      { label: 'Playground',    href: '/playground',    icon: MessageSquare },
      { label: 'Conversations', href: '/conversations', icon: History },
    ],
  },
  {
    label: 'Observe',
    items: [
      { label: 'Runs & Traces', href: '/runs',          icon: Activity },
      { label: 'Evals',         href: '/evals',         icon: FlaskConical },
      { label: 'Memory',        href: '/memory',        icon: Brain },
      { label: 'Usage',         href: '/usage',         icon: BarChart2 },
      { label: 'Latency',       href: '/observability', icon: Timer },
    ],
  },
  {
    label: 'Settings',
    items: [
      { label: 'Providers',   href: '/settings/providers',   icon: Key },
      { label: 'Claude Code', href: '/settings/claude-code', icon: SquareTerminal },
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
      { label: 'Claude Code', href: '/admin/claude-code', icon: SquareTerminal },
      { label: 'Users',       href: '/admin/users',       icon: Users },
      { label: 'Workspaces',  href: '/admin/workspaces',  icon: Building2 },
      { label: 'Policies',    href: '/admin/policies',    icon: Shield },
      { label: 'Audit Logs',  href: '/admin/audit-logs',  icon: ClipboardList },
      { label: 'Service Logs', href: '/admin/service-logs', icon: SquareTerminal },
    ],
  },
]

function NavTip({ label, collapsed, children }: { label: string; collapsed: boolean; children: React.ReactNode }) {
  if (!collapsed) return <>{children}</>
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          side="right"
          sideOffset={6}
          className="z-50 bg-foreground text-background text-[11px] px-2 py-1 rounded shadow-lg select-none"
        >
          {label}
          <TooltipPrimitive.Arrow className="fill-[hsl(var(--foreground))]" />
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  )
}

function NavGroupSection({
  group, pathname, collapsed, isOpen, onToggle,
}: {
  group: NavGroup
  pathname: string
  collapsed: boolean
  isOpen: boolean
  onToggle: () => void
}) {
  return (
    <div className="mb-1">
      {collapsed
        ? <div className="pt-3 border-t border-border mt-1 first:border-0 first:mt-0" />
        : (
          <button
            onClick={onToggle}
            className="w-full flex items-center gap-1 px-4 pt-3 pb-1 text-[10px] font-mono font-medium text-faint hover:text-muted-foreground uppercase tracking-[0.14em] transition-colors"
          >
            {isOpen ? <ChevronDown className="w-2.5 h-2.5 flex-shrink-0" /> : <ChevronRight className="w-2.5 h-2.5 flex-shrink-0" />}
            {group.label}
          </button>
        )
      }
      {(collapsed || isOpen) && group.items.map((item) => {
        const active = pathname === item.href || pathname.startsWith(item.href + '/')
        return (
          <NavTip key={item.href + item.label} label={item.label} collapsed={collapsed}>
            <Link
              href={item.href}
              className={cn(
                'relative flex items-center gap-2.5 py-1.5 text-[13px] transition-colors',
                collapsed ? 'justify-center mx-1.5 rounded-md px-0 py-2' : 'mx-2 px-2.5 rounded-lg',
                active
                  ? 'text-accent dark:text-accent-bright bg-accent/[0.09] dark:bg-accent-bright/[0.12] font-medium'
                  : 'text-muted-foreground hover:text-foreground hover:bg-muted'
              )}
            >
              {active && !collapsed && (
                <span className="absolute -left-2 top-1.5 bottom-1.5 w-[2.5px] rounded-full bg-accent dark:bg-accent-bright" />
              )}
              <item.icon className="w-3.5 h-3.5 flex-shrink-0" />
              {!collapsed && item.label}
            </Link>
          </NavTip>
        )
      })}
    </div>
  )
}

export function Sidebar({ isAdmin = false, onClose }: { isAdmin?: boolean; onClose?: () => void }) {
  const pathname = usePathname()
  const user = useAuthStore((s) => s.user)
  const userIsAdmin = user?.is_admin ?? false
  const groups = isAdmin ? adminNav : userNav

  const [collapsed, setCollapsed] = useState(false)
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({})

  useEffect(() => {
    setCollapsed(localStorage.getItem('sidebar-collapsed') === 'true')
    try {
      setCollapsedGroups(JSON.parse(localStorage.getItem('sidebar-group-collapsed') ?? '{}'))
    } catch {
      setCollapsedGroups({})
    }
  }, [])

  const toggle = () => {
    setCollapsed((v) => {
      localStorage.setItem('sidebar-collapsed', String(!v))
      return !v
    })
  }

  const toggleGroup = (label: string) => {
    setCollapsedGroups((prev) => {
      const next = { ...prev, [label]: !prev[label] }
      localStorage.setItem('sidebar-group-collapsed', JSON.stringify(next))
      return next
    })
  }

  // A group containing the active route is always shown expanded, regardless
  // of its persisted collapsed state, so navigating never hides the page
  // you're currently on.
  const activeGroupLabel = groups.find((g) =>
    g.items.some((item) => pathname === item.href || pathname.startsWith(item.href + '/'))
  )?.label

  return (
    <TooltipPrimitive.Provider delayDuration={400}>
      <aside
        className={cn(
          'flex flex-col h-full bg-surface border-r border-border transition-all duration-200 flex-shrink-0 overflow-hidden',
          collapsed ? 'w-12' : 'w-52'
        )}
      >
        {/* Logo */}
        <div className={cn(
          'flex items-center border-b border-border flex-shrink-0',
          collapsed ? 'justify-center py-3 px-0' : 'px-4 py-3 gap-2.5'
        )}>
          <Link
            href="/dashboard"
            className="flex items-center gap-2.5 flex-1 min-w-0 hover:opacity-80 transition-opacity"
            aria-label="Open dashboard"
          >
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src="/icon.svg" alt="Agent Nexus" className="w-7 h-7 rounded-lg flex-shrink-0 shadow-sm" />
            {!collapsed && (
              <div className="min-w-0">
                <div className="flex items-center gap-1.5">
                  <span className="text-sm font-semibold text-foreground truncate tracking-tight">Agent Nexus</span>
                  {isAdmin && (
                    <span className="text-[9px] bg-warn/15 text-warn px-1.5 py-0.5 rounded font-mono font-medium flex-shrink-0">ADMIN</span>
                  )}
                </div>
              </div>
            )}
          </Link>
          {/* Close button — mobile drawer only */}
          {onClose && !collapsed && (
            <button
              onClick={onClose}
              className="ml-auto p-1 rounded text-muted-foreground hover:text-foreground flex-shrink-0"
              aria-label="Close navigation"
            >
              <X className="w-4 h-4" />
            </button>
          )}
        </div>

        {/* Workspace switcher */}
        {!isAdmin && <WorkspaceSwitcher collapsed={collapsed} />}

        {/* Nav */}
        <nav className="flex-1 overflow-y-auto py-2">
          {groups.map((group) => (
            <NavGroupSection
              key={group.label}
              group={group}
              pathname={pathname}
              collapsed={collapsed}
              isOpen={group.label === activeGroupLabel || !collapsedGroups[group.label]}
              onToggle={() => toggleGroup(group.label)}
            />
          ))}
        </nav>

        {/* Admin link — only for admin users in the regular sidebar */}
        {!isAdmin && userIsAdmin && (
          <div className="border-t border-border p-1.5">
            <NavTip label="Admin dashboard" collapsed={collapsed}>
              <Link
                href="/admin/overview"
                className={cn(
                  'flex items-center gap-2 px-3 py-2 rounded-lg text-[12px] transition-colors w-full',
                  collapsed && 'justify-center px-0',
                  pathname.startsWith('/admin')
                    ? 'text-warn bg-warn/10 font-medium'
                    : 'text-muted-foreground hover:text-warn hover:bg-muted'
                )}
              >
                <ShieldCheck className="w-3.5 h-3.5 flex-shrink-0" />
                {!collapsed && 'Admin dashboard'}
              </Link>
            </NavTip>
          </div>
        )}

        {/* Back to app link when in admin sidebar */}
        {isAdmin && (
          <div className="border-t border-border p-1.5">
            <NavTip label="Back to app" collapsed={collapsed}>
              <Link
                href="/dashboard"
                className={cn(
                  'flex items-center gap-2 px-3 py-2 rounded-lg text-[12px] text-muted-foreground hover:text-foreground hover:bg-muted transition-colors w-full',
                  collapsed && 'justify-center px-0'
                )}
              >
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img src="/icon.svg" alt="" className="w-3.5 h-3.5 rounded flex-shrink-0" />
                {!collapsed && 'Back to app'}
              </Link>
            </NavTip>
          </div>
        )}

        {/* Collapse toggle */}
        <div className={cn('border-t border-border p-1.5')}>
          <NavTip label="Expand sidebar" collapsed={collapsed}>
            <button
              onClick={toggle}
              className={cn(
                'flex items-center gap-2 px-3 py-2 rounded-lg text-[12px] text-muted-foreground hover:text-foreground hover:bg-muted transition-colors w-full',
                collapsed && 'justify-center px-0'
              )}
            >
              {collapsed
                ? <PanelLeftOpen className="w-3.5 h-3.5 flex-shrink-0" />
                : <><PanelLeftClose className="w-3.5 h-3.5 flex-shrink-0" /><span>Collapse</span></>
              }
            </button>
          </NavTip>
        </div>
      </aside>
    </TooltipPrimitive.Provider>
  )
}
