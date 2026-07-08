'use client'

/**
 * Sparkline — a compact area+line trend chart. Pure SVG so it's crisp and
 * theme-aware (stroke/fill use currentColor via the `tone` class on the wrapper).
 * Renders nothing for <2 points so we never fake a trend from thin data.
 */
export function Sparkline({
  data,
  className = '',
  height = 34,
}: {
  data: number[]
  className?: string
  height?: number
}) {
  if (!data || data.length < 2) return null
  const w = 100 // viewBox width; scales to container
  const h = height
  const pad = 3
  const min = Math.min(...data)
  const max = Math.max(...data)
  const rng = max - min || 1
  const x = (i: number) => pad + (i / (data.length - 1)) * (w - 2 * pad)
  const y = (v: number) => h - pad - ((v - min) / rng) * (h - 2 * pad)
  const line = data.map((v, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' ')
  const area = `${line} L${x(data.length - 1).toFixed(1)},${h} L${x(0).toFixed(1)},${h} Z`
  const gid = `spk-${Math.random().toString(36).slice(2, 8)}`
  return (
    <svg
      viewBox={`0 0 ${w} ${h}`}
      preserveAspectRatio="none"
      className={`w-full ${className}`}
      style={{ height }}
      aria-hidden="true"
    >
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="currentColor" stopOpacity="0.22" />
          <stop offset="100%" stopColor="currentColor" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={area} fill={`url(#${gid})`} />
      <path d={line} fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
      <circle cx={x(data.length - 1)} cy={y(data[data.length - 1])} r="2" fill="currentColor" />
    </svg>
  )
}
