import type { ReactNode } from 'react'

/**
 * DataTable — the shared table shell used across list screens. Mono uppercase
 * column headers, hover rows, horizontal scroll on narrow screens. Matches the
 * mock's table treatment so every list reads as one system.
 */
export function DataTable({ columns, children, minWidth = 560 }: { columns: string[]; children: ReactNode; minWidth?: number }) {
  return (
    <div className="rounded-xl border border-border bg-surface shadow-card overflow-x-auto">
      <table className="w-full text-[13px]" style={{ minWidth }}>
        <thead>
          <tr className="border-b border-border">
            {columns.map((c, i) => (
              <th key={i} className="text-left font-mono text-[10px] uppercase tracking-[0.09em] text-faint font-semibold px-3.5 py-3 whitespace-nowrap">
                {c}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  )
}

/** A hover row; pass onClick to make it interactive. */
export function Row({ children, onClick }: { children: ReactNode; onClick?: () => void }) {
  return (
    <tr
      onClick={onClick}
      className={`border-b border-border last:border-b-0 transition-colors ${onClick ? 'cursor-pointer hover:bg-muted' : ''}`}
    >
      {children}
    </tr>
  )
}

export function Cell({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <td className={`px-3.5 py-2.5 align-middle whitespace-nowrap ${className}`}>{children}</td>
}
