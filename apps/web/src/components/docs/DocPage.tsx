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
    <div className="doc-content">
      <style>{`
        .doc-content h1 { font-size: 1.875rem; font-weight: 700; color: #111827; margin: 0 0 0.5rem; letter-spacing: -0.02em; line-height: 1.2; }
        .doc-content h2 { font-size: 1.125rem; font-weight: 600; color: #111827; margin: 2.5rem 0 0.75rem; padding-top: 2rem; border-top: 1px solid #f3f4f6; }
        .doc-content h2:first-of-type { margin-top: 2rem; padding-top: 0; border-top: none; }
        .doc-content h3 { font-size: 0.9375rem; font-weight: 600; color: #374151; margin: 1.75rem 0 0.5rem; }
        .doc-content p { font-size: 0.9375rem; line-height: 1.7; color: #4b5563; margin: 0 0 1rem; }
        .doc-content ul, .doc-content ol { padding-left: 1.5rem; margin: 0 0 1rem; }
        .doc-content li { font-size: 0.9375rem; line-height: 1.7; color: #4b5563; margin-bottom: 0.35rem; }
        .doc-content code { font-family: 'Fira Code', 'JetBrains Mono', 'Consolas', monospace; font-size: 0.8125rem; background: #f1f0ff; color: #7c3aed; padding: 0.15em 0.4em; border-radius: 4px; }
        .doc-content pre { background: #0d0d14; border-radius: 10px; padding: 1.25rem 1.5rem; overflow-x: auto; margin: 1.25rem 0; border: 1px solid rgba(255,255,255,0.06); }
        .doc-content pre code { background: none; color: #e2e8f0; padding: 0; font-size: 0.8125rem; line-height: 1.7; }
        .doc-content strong { font-weight: 600; color: #374151; }
        .doc-content a { color: #7c3aed; text-decoration: none; }
        .doc-content a:hover { text-decoration: underline; }
        .doc-content table { width: 100%; border-collapse: collapse; font-size: 0.875rem; margin: 1rem 0 1.5rem; }
        .doc-content th { text-align: left; padding: 0.5rem 0.75rem; font-weight: 600; color: #374151; background: #f9fafb; border-bottom: 1px solid #e5e7eb; }
        .doc-content td { padding: 0.5rem 0.75rem; color: #4b5563; border-bottom: 1px solid #f3f4f6; vertical-align: top; }
        .doc-content td code { white-space: nowrap; }
      `}</style>

      <h1>{title}</h1>
      {subtitle && (
        <p style={{ fontSize: '1.0625rem', color: '#6b7280', marginBottom: '2rem', lineHeight: 1.6 }}>
          {subtitle}
        </p>
      )}

      {children}
    </div>
  )
}

// A styled callout box (tip / warning / info)
export function Callout({ type = 'info', children }: { type?: 'info' | 'tip' | 'warning'; children: ReactNode }) {
  const styles = {
    info:    { bg: '#f0f0ff', border: '#c7c5f0', text: '#3d3999' },
    tip:     { bg: '#f0fdf4', border: '#bbf7d0', text: '#166534' },
    warning: { bg: '#fffbeb', border: '#fde68a', text: '#92400e' },
  }[type]

  return (
    <div
      style={{
        background: styles.bg,
        border: `1px solid ${styles.border}`,
        borderRadius: 8,
        padding: '0.875rem 1.125rem',
        margin: '1.25rem 0',
        fontSize: '0.875rem',
        lineHeight: 1.65,
        color: styles.text,
      }}
    >
      {children}
    </div>
  )
}

// A pill-style badge used inline
export function Badge({ label, color = 'purple' }: { label: string; color?: 'purple' | 'blue' | 'green' | 'amber' | 'red' | 'gray' }) {
  const map = {
    purple: { bg: '#f1f0ff', text: '#7c3aed' },
    blue:   { bg: '#eff6ff', text: '#1d4ed8' },
    green:  { bg: '#f0fdf4', text: '#166534' },
    amber:  { bg: '#fffbeb', text: '#92400e' },
    red:    { bg: '#fff1f2', text: '#be123c' },
    gray:   { bg: '#f9fafb', text: '#6b7280' },
  }[color]
  return (
    <span style={{
      background: map.bg, color: map.text,
      fontSize: '0.75rem', fontWeight: 600,
      padding: '0.15em 0.55em', borderRadius: 4,
      fontFamily: 'inherit', letterSpacing: '0.01em',
    }}>
      {label}
    </span>
  )
}

// A grid of concept cards used on the overview page
export function ConceptGrid({ items }: {
  items: { title: string; href: string; desc: string; badge?: string }[]
}) {
  return (
    <div style={{ display: 'grid', gap: '0.75rem', margin: '1.25rem 0 2rem' }}>
      {items.map((c) => (
        <a
          key={c.href}
          href={c.href}
          style={{
            display: 'flex', gap: '1rem', alignItems: 'flex-start',
            padding: '1rem 1.125rem', borderRadius: 10,
            border: '1px solid #e5e7eb', background: '#fff',
            textDecoration: 'none', transition: 'border-color 0.15s, box-shadow 0.15s',
          }}
          onMouseEnter={(e) => { (e.currentTarget as HTMLElement).style.borderColor = '#7c3aed'; (e.currentTarget as HTMLElement).style.boxShadow = '0 0 0 3px rgba(83,74,183,0.08)' }}
          onMouseLeave={(e) => { (e.currentTarget as HTMLElement).style.borderColor = '#e5e7eb'; (e.currentTarget as HTMLElement).style.boxShadow = 'none' }}
        >
          {c.badge && (
            <span style={{
              flexShrink: 0, marginTop: 2,
              background: '#f1f0ff', color: '#7c3aed',
              fontSize: '0.7rem', fontWeight: 700,
              padding: '0.2em 0.55em', borderRadius: 4,
              whiteSpace: 'nowrap', letterSpacing: '0.03em',
            }}>
              {c.badge}
            </span>
          )}
          <div>
            <div style={{ fontWeight: 600, fontSize: '0.9375rem', color: '#111827', marginBottom: 2 }}>{c.title}</div>
            <div style={{ fontSize: '0.875rem', color: '#6b7280', lineHeight: 1.5 }}>{c.desc}</div>
          </div>
        </a>
      ))}
    </div>
  )
}
