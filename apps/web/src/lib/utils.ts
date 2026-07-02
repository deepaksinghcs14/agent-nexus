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
    case 'low':      return 'bg-blue-50 text-blue-800 border-blue-200'
    case 'medium':   return 'bg-amber-50 text-amber-800 border-amber-200'
    case 'high':     return 'bg-orange-50 text-orange-800 border-orange-200'
    case 'critical': return 'bg-red-50 text-red-800 border-red-200'
    default:         return 'bg-gray-50 text-gray-700 border-gray-200'
  }
}

export function statusColor(status: string): string {
  switch (status) {
    case 'success':
    case 'connected':
    case 'active':      return 'bg-green-50 text-green-800'
    case 'running':
    case 'syncing':     return 'bg-amber-50 text-amber-800'
    case 'failed':
    case 'error':       return 'bg-red-50 text-red-800'
    case 'approval_wait': return 'bg-purple-50 text-purple-800'
    case 'session_wait':  return 'bg-indigo-50 text-indigo-800'
    case 'pending':
    case 'disconnected':
    default:            return 'bg-gray-100 text-gray-600'
  }
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
