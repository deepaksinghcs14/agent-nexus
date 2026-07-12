import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

export function formatCost(usd: number): string {
  return `$${usd.toFixed(4)}`
}

export function riskColor(risk: string): string {
  switch (risk) {
    case 'low':      return 'bg-info/10 text-info border-info/30'
    case 'medium':   return 'bg-warn/10 text-warn border-warn/30'
    case 'high':     return 'bg-warn/15 text-warn border-warn/40'
    case 'critical': return 'bg-crit/10 text-crit border-crit/30'
    default:         return 'bg-muted text-muted-foreground border-border'
  }
}

export function statusColor(status: string): string {
  switch (status) {
    case 'success':
    case 'connected':
    case 'active':      return 'bg-good/10 text-good border border-good/30'
    case 'running':
    case 'syncing':     return 'bg-warn/10 text-warn border border-warn/30'
    case 'failed':
    case 'error':       return 'bg-crit/10 text-crit border border-crit/30'
    case 'approval_wait':
    case 'session_wait': return 'bg-accent/10 text-accent dark:text-accent-bright border border-accent/25'
    case 'pending':
    case 'disconnected':
    default:            return 'bg-muted text-muted-foreground border border-border'
  }
}

export function formatDate(s: string | null | undefined): string {
  if (!s) return '—'
  return new Date(s).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' })
}

export function relativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  const secs = Math.floor(diff / 1000)
  if (secs < 60) return `${secs}s ago`
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.floor(hrs / 24)}d ago`
}
