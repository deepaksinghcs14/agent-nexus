import type { EvalRun } from '@/types'

export function RunStatusBadge({ status }: { status: EvalRun['status'] }) {
  const cls = {
    pending: 'bg-muted text-muted-foreground',
    running: 'bg-info/10 text-info',
    completed: 'bg-good/10 text-good',
    failed: 'bg-crit/10 text-crit',
  }[status] ?? 'bg-muted text-muted-foreground'
  return <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${cls}`}>{status}</span>
}
