import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const badgeVariants = cva(
  'inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-medium',
  {
    variants: {
      variant: {
        default: 'bg-accent-light text-accent border-transparent',
        outline: 'border-border text-foreground',
        success: 'bg-green-50 text-green-800 border-transparent',
        warning: 'bg-amber-50 text-amber-800 border-transparent',
        destructive: 'bg-red-50 text-red-800 border-transparent',
        secondary: 'bg-muted text-muted-foreground border-transparent',
      },
    },
    defaultVariants: { variant: 'default' },
  }
)

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />
}
