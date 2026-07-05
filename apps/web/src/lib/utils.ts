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
    case 'low':      return 'bg-blue-50 dark:bg-blue-500/10 text-blue-800 dark:text-blue-300 border-blue-200 dark:border-blue-800'
    case 'medium':   return 'bg-amber-50 dark:bg-amber-500/10 text-amber-800 dark:text-amber-300 border-amber-200 dark:border-amber-800'
    case 'high':     return 'bg-orange-50 dark:bg-orange-500/10 text-orange-800 dark:text-orange-300 border-orange-200 dark:border-orange-800'
    case 'critical': return 'bg-red-50 dark:bg-red-500/10 text-red-800 dark:text-red-300 border-red-200 dark:border-red-800'
    default:         return 'bg-gray-50 dark:bg-gray-800/60 text-gray-700 dark:text-gray-300 border-gray-200 dark:border-gray-700'
  }
}

export function statusColor(status: string): string {
  switch (status) {
    case 'success':
    case 'connected':
    case 'active':      return 'bg-green-50 dark:bg-green-500/10 text-green-800 dark:text-green-300'
    case 'running':
    case 'syncing':     return 'bg-amber-50 dark:bg-amber-500/10 text-amber-800 dark:text-amber-300'
    case 'failed':
    case 'error':       return 'bg-red-50 dark:bg-red-500/10 text-red-800 dark:text-red-300'
    case 'approval_wait': return 'bg-purple-50 dark:bg-purple-500/10 text-purple-800 dark:text-purple-300'
    case 'session_wait':  return 'bg-indigo-50 dark:bg-indigo-500/10 text-indigo-800 dark:text-indigo-300'
    case 'pending':
    case 'disconnected':
    default:            return 'bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-400'
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
