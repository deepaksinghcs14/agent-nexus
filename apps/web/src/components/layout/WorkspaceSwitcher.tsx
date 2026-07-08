'use client'

import { useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { ChevronDown, Check, Plus, Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/store/auth'
import { workspacesAPI } from '@/lib/api'
import type { WorkspaceType } from '@/types'

const TYPE_LABELS: Record<WorkspaceType, string> = {
  personal: 'Personal',
  team: 'Team',
  organization: 'Org',
  project: 'Project',
  sandbox: 'Sandbox',
}

const TYPE_COLORS: Record<WorkspaceType, string> = {
  personal: 'bg-accent/15 text-accent dark:text-accent-bright',
  team: 'bg-info/15 text-info',
  organization: 'bg-good/15 text-good',
  project: 'bg-warn/15 text-warn',
  sandbox: 'bg-muted text-muted-foreground',
}

export function WorkspaceSwitcher({ collapsed = false }: { collapsed?: boolean }) {
  const router = useRouter()
  const triggerRef = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(false)
  const [dropdownRect, setDropdownRect] = useState<DOMRect | null>(null)
  const [switching, setSwitching] = useState<string | null>(null)

  const workspace = useAuthStore((s) => s.workspace)
  const workspaces = useAuthStore((s) => s.workspaces)
  const switchWorkspace = useAuthStore((s) => s.switchWorkspace)

  const wsType = (workspace?.workspace_type ?? 'personal') as WorkspaceType

  const handleToggle = () => {
    if (!open && triggerRef.current) {
      setDropdownRect(triggerRef.current.getBoundingClientRect())
    }
    setOpen((v) => !v)
  }

  const handleClose = () => setOpen(false)

  const handleSwitch = async (targetId: string) => {
    if (targetId === workspace?.id || switching) return
    setSwitching(targetId)
    try {
      const { access_token, workspace_id } = await workspacesAPI.switch(targetId)
      switchWorkspace(workspace_id, access_token)
      handleClose()
      router.refresh()
    } catch {
      // stays on current workspace
    } finally {
      setSwitching(null)
    }
  }

  const initial = (workspace?.display_name ?? '?')[0].toUpperCase()

  return (
    <div className="border-b border-border">
      <button
        ref={triggerRef}
        onClick={handleToggle}
        className={cn(
          'w-full flex items-center hover:bg-muted transition-colors text-left',
          collapsed ? 'justify-center py-2.5 px-0' : 'gap-2 px-4 py-2.5'
        )}
      >
        {collapsed ? (
          <div className={cn('w-6 h-6 rounded flex items-center justify-center text-[11px] font-bold flex-shrink-0', TYPE_COLORS[wsType])}>
            {initial}
          </div>
        ) : (
          <>
            <div className="flex-1 min-w-0">
              <p className="text-[12px] font-medium text-foreground truncate leading-tight">
                {workspace?.display_name ?? 'Loading…'}
              </p>
              <span className={cn('inline-block text-[9px] font-semibold px-1.5 py-0.5 rounded mt-0.5 uppercase tracking-wider', TYPE_COLORS[wsType])}>
                {TYPE_LABELS[wsType]}
              </span>
            </div>
            <ChevronDown className={cn('w-3.5 h-3.5 text-muted-foreground flex-shrink-0 transition-transform', open && 'rotate-180')} />
          </>
        )}
      </button>

      {open && dropdownRect && (
        <>
          {/* backdrop — fixed so it sits above overflow-hidden parents */}
          <div className="fixed inset-0 z-40" onClick={handleClose} />
          {/* dropdown — fixed with coords from trigger rect */}
          <div
            className="fixed z-50 bg-surface border border-border rounded-xl shadow-card overflow-hidden"
            style={{
              top: dropdownRect.bottom + 4,
              left: dropdownRect.left,
              width: dropdownRect.width,
            }}
          >
            <p className="px-3 pt-2.5 pb-1 text-[9px] font-semibold text-faint uppercase tracking-widest">
              Your workspaces
            </p>
            <div className="max-h-60 overflow-y-auto">
              {workspaces.length === 0 && (
                <p className="px-3 py-3 text-[11px] text-muted-foreground">No workspaces yet</p>
              )}
              {workspaces.map((ws) => {
                const t = (ws.workspace_type ?? 'personal') as WorkspaceType
                const isActive = ws.id === workspace?.id
                const isLoading = switching === ws.id
                return (
                  <button
                    key={ws.id}
                    onClick={() => handleSwitch(ws.id)}
                    disabled={isActive || !!switching}
                    className={cn(
                      'w-full flex items-center gap-2.5 px-3 py-2 text-left transition-colors',
                      isActive ? 'bg-muted cursor-default' : 'hover:bg-muted cursor-pointer'
                    )}
                  >
                    <div className="flex-1 min-w-0">
                      <p className={cn('text-[12px] font-medium truncate', isActive ? 'text-foreground' : 'text-muted-foreground')}>
                        {ws.display_name}
                      </p>
                      <div className="flex items-center gap-1.5 mt-0.5">
                        <span className={cn('text-[9px] font-semibold px-1 py-0.5 rounded uppercase tracking-wider', TYPE_COLORS[t])}>
                          {TYPE_LABELS[t]}
                        </span>
                        <span className="text-[10px] text-faint capitalize">{ws.role}</span>
                      </div>
                    </div>
                    {isLoading && <Loader2 className="w-3 h-3 text-muted-foreground animate-spin flex-shrink-0" />}
                    {isActive && !isLoading && <Check className="w-3 h-3 text-accent dark:text-accent-bright flex-shrink-0" />}
                  </button>
                )
              })}
            </div>
            <div className="border-t border-border p-1.5">
              <Link
                href="/settings/workspace/new"
                onClick={handleClose}
                className="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-[11px] text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
              >
                <Plus className="w-3 h-3" />
                New workspace
              </Link>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
