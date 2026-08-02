'use client'

import { AlertCircle, CheckCircle2, X } from 'lucide-react'
import { useToastStore } from '@/store/toast'
import { cn } from '@/lib/utils'

/**
 * Toaster — renders the global toast queue (see @/store/toast). Mounted once
 * in providers.tsx; errors from every mutation land here automatically via
 * the QueryClient's MutationCache, plus any explicit toast() call.
 */
export function Toaster() {
  const toasts = useToastStore((s) => s.toasts)
  const dismiss = useToastStore((s) => s.dismiss)
  if (toasts.length === 0) return null
  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 w-full max-w-sm sm:max-w-sm">
      {toasts.map((t) => (
        <button
          key={t.id}
          type="button"
          onClick={() => dismiss(t.id)}
          className={cn(
            'flex items-start gap-2 rounded-lg border px-3.5 py-2.5 text-xs text-left shadow-card',
            t.variant === 'error'
              ? 'bg-crit/10 text-crit border-crit/30'
              : 'bg-good/10 text-good border-good/30'
          )}
        >
          {t.variant === 'error' ? (
            <AlertCircle className="w-3.5 h-3.5 mt-0.5 shrink-0" />
          ) : (
            <CheckCircle2 className="w-3.5 h-3.5 mt-0.5 shrink-0" />
          )}
          <span className="flex-1 leading-relaxed">{t.message}</span>
          <X className="w-3.5 h-3.5 mt-0.5 shrink-0 opacity-60" />
        </button>
      ))}
    </div>
  )
}
