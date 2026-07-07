'use client'

import Link from 'next/link'
import { Sparkline } from './Sparkline'

type Tone = 'accent' | 'good' | 'warn' | 'crit'

const toneText: Record<Tone, string> = {
  accent: 'text-accent dark:text-accent-bright',
  good: 'text-good',
  warn: 'text-warn',
  crit: 'text-crit',
}

/**
 * KpiTile — an instrument-panel metric card: mono eyebrow, big tabular value,
 * optional delta, optional sparkline. Matches the mission-control mock. When
 * `series` is omitted no chart is drawn (we don't fabricate trends).
 */
export function KpiTile({
  label,
  value,
  unit,
  delta,
  series,
  tone = 'accent',
  href,
}: {
  label: string
  value: string | number
  unit?: string
  delta?: string
  series?: number[]
  tone?: Tone
  href?: string
}) {
  const up = delta ? !delta.trim().startsWith('-') : true
  const inner = (
    <>
      <span className="eyebrow block mb-2.5">{label}</span>
      <div className="flex items-baseline justify-between gap-2">
        <div className="text-[26px] font-bold tracking-tight leading-none tabular-nums text-foreground">
          {unit === '$' && <span className="text-[15px] font-semibold text-muted-foreground">$</span>}
          {value}
          {unit && unit !== '$' && <span className="text-[15px] font-semibold text-muted-foreground ml-0.5">{unit}</span>}
        </div>
        {delta && (
          <div className={`font-mono text-[11.5px] font-semibold ${up ? 'text-good' : 'text-crit'}`}>{delta}</div>
        )}
      </div>
      {series && series.length >= 2 && (
        <div className={`mt-2.5 ${toneText[tone]}`}>
          <Sparkline data={series} />
        </div>
      )}
    </>
  )
  const cls = 'block rounded-xl border border-border bg-surface shadow-card px-4 pt-3.5 pb-3 transition-colors'
  return href
    ? <Link href={href} className={`${cls} hover:border-border-strong`}>{inner}</Link>
    : <div className={cls}>{inner}</div>
}
