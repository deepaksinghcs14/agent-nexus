'use client'

import type { ReactNode } from 'react'

interface DocPageProps {
  title: string
  subtitle?: string
  children: ReactNode
}

// Shared layout wrapper for a single doc page — consistent heading, spacing, typography.
export function DocPage({ title, subtitle, children }: DocPageProps) {
  return (
    <div className="doc-content max-w-3xl mx-auto px-4 sm:px-6 py-6">
      <style>{`
        .doc-content h1 { font-size: 1.5rem; font-weight: 700; color: hsl(var(--foreground)); margin: 0 0 0.5rem; letter-spacing: -0.02em; line-height: 1.2; }
        @media (min-width: 640px) { .doc-content h1 { font-size: 1.875rem; } }
        .doc-content h2 { font-size: 1.125rem; font-weight: 600; color: hsl(var(--foreground)); margin: 2.5rem 0 0.75rem; padding-top: 2rem; border-top: 1px solid hsl(var(--border)); }
        .doc-content h2:first-of-type { margin-top: 2rem; padding-top: 0; border-top: none; }
        .doc-content h3 { font-size: 0.9375rem; font-weight: 600; color: hsl(var(--foreground)); margin: 1.75rem 0 0.5rem; }
        .doc-content p { font-size: 0.9375rem; line-height: 1.7; color: hsl(var(--muted-foreground)); margin: 0 0 1rem; }
        .doc-content ul, .doc-content ol { padding-left: 1.5rem; margin: 0 0 1rem; }
        .doc-content li { font-size: 0.9375rem; line-height: 1.7; color: hsl(var(--muted-foreground)); margin-bottom: 0.35rem; }
        .doc-content code { font-family: var(--font-mono), 'JetBrains Mono', monospace; font-size: 0.8125rem; background: hsl(var(--muted)); color: hsl(var(--ring)); padding: 0.15em 0.4em; border-radius: 4px; }
        .doc-content pre { background: #0b0f14; border-radius: 10px; padding: 1rem 1.25rem; overflow-x: auto; -webkit-overflow-scrolling: touch; margin: 1.25rem 0; border: 1px solid hsl(var(--border)); }
        @media (min-width: 640px) { .doc-content pre { padding: 1.25rem 1.5rem; } }
        .doc-content pre code { background: none; color: #cdd9e5; padding: 0; font-size: 0.8125rem; line-height: 1.7; white-space: pre; }
        .doc-content strong { font-weight: 600; color: hsl(var(--foreground)); }
        .doc-content a { color: hsl(var(--ring)); text-decoration: none; }
        .doc-content a:hover { text-decoration: underline; }
        .doc-content .table-scroll { display: block; overflow-x: auto; -webkit-overflow-scrolling: touch; margin: 1rem 0 1.5rem; }
        .doc-content table { width: 100%; border-collapse: collapse; font-size: 0.875rem; }
        /* Bare tables (most doc pages skip .table-scroll) must scroll on narrow
           screens rather than pushing the whole page wider than the viewport. */
        .doc-content > table { display: block; overflow-x: auto; -webkit-overflow-scrolling: touch; margin: 1rem 0 1.5rem; }
        .doc-content th { text-align: left; padding: 0.5rem 0.75rem; font-weight: 600; color: hsl(var(--foreground)); background: hsl(var(--surface-2)); border-bottom: 1px solid hsl(var(--border)); white-space: nowrap; }
        .doc-content td { padding: 0.5rem 0.75rem; color: hsl(var(--muted-foreground)); border-bottom: 1px solid hsl(var(--border)); vertical-align: top; }
        .doc-content td code { white-space: nowrap; }
      `}</style>

      <h1>{title}</h1>
      {subtitle && (
        <p className="text-muted-foreground" style={{ fontSize: '1.0625rem', marginBottom: '2rem', lineHeight: 1.6 }}>
          {subtitle}
        </p>
      )}

      {children}
    </div>
  )
}

// A styled callout box (tip / warning / info)
export function Callout({ type = 'info', children }: { type?: 'info' | 'tip' | 'warning'; children: ReactNode }) {
  const cls = {
    info:    'bg-accent/10 border-accent/30 text-foreground',
    tip:     'bg-good/10 border-good/30 text-foreground',
    warning: 'bg-warn/10 border-warn/30 text-foreground',
  }[type]
  return (
    <div className={`border rounded-lg my-5 px-[1.125rem] py-3.5 text-sm leading-relaxed ${cls}`}>
      {children}
    </div>
  )
}

// A pill-style badge used inline
export function Badge({ label, color = 'purple' }: { label: string; color?: 'purple' | 'blue' | 'green' | 'amber' | 'red' | 'gray' }) {
  const cls = {
    purple: 'bg-accent/10 text-accent dark:text-accent-bright',
    blue:   'bg-info/10 text-info',
    green:  'bg-good/10 text-good',
    amber:  'bg-warn/10 text-warn',
    red:    'bg-crit/10 text-crit',
    gray:   'bg-muted text-muted-foreground',
  }[color]
  return (
    <span className={`text-xs font-semibold px-1.5 py-0.5 rounded tracking-wide ${cls}`}>
      {label}
    </span>
  )
}

// A grid of concept cards used on the overview page
export function ConceptGrid({ items }: {
  items: { title: string; href: string; desc: string; badge?: string }[]
}) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 my-5 mb-8">
      {items.map((c) => (
        <a
          key={c.href}
          href={c.href}
          className="flex gap-4 items-start p-4 rounded-xl border border-border bg-surface shadow-card no-underline transition-[border-color,box-shadow] duration-150 hover:border-accent/50 hover:shadow-[0_0_0_3px_rgba(83,74,183,0.08)]"
        >
          {c.badge && (
            <span className="shrink-0 mt-0.5 bg-accent/10 text-accent dark:text-accent-bright text-[0.7rem] font-bold px-1.5 py-0.5 rounded whitespace-nowrap tracking-wide">
              {c.badge}
            </span>
          )}
          <div>
            <div className="font-semibold text-[0.9375rem] text-foreground mb-0.5">{c.title}</div>
            <div className="text-sm text-muted-foreground leading-snug">{c.desc}</div>
          </div>
        </a>
      ))}
    </div>
  )
}
