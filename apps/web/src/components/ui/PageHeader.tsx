import type { ReactNode } from 'react'

/**
 * PageHeader — the standard screen header: mono eyebrow, balanced title,
 * optional subtitle, and a right-aligned actions slot. Used across all screens
 * so headings read as one system.
 */
export function PageHeader({
  eyebrow,
  title,
  subtitle,
  actions,
}: {
  eyebrow?: string
  title: string
  subtitle?: ReactNode
  actions?: ReactNode
}) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-4 mb-6">
      <div className="min-w-0">
        {eyebrow && <span className="eyebrow block">{eyebrow}</span>}
        <h1 className="text-[22px] font-bold tracking-tight text-foreground mt-1 text-balance">{title}</h1>
        {subtitle && <div className="text-[13px] text-muted-foreground mt-1 max-w-[62ch]">{subtitle}</div>}
      </div>
      {actions && <div className="flex items-center gap-2 flex-shrink-0">{actions}</div>}
    </div>
  )
}
