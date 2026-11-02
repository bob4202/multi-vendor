import { formatPrice } from "@/lib/format"
import { cn } from "@/lib/utils"

interface PriceProps {
  pence: number | null
  // Shown in place of the price when it's null (unpriced product).
  fallback?: string
  className?: string
}

function Price({ pence, fallback = "No price set", className }: PriceProps) {
  const formatted = formatPrice(pence)

  if (formatted === null) {
    return (
      <span
        className={cn("text-sm font-normal text-muted-foreground", className)}
      >
        {fallback}
      </span>
    )
  }
  return <span className={cn("tabular-nums", className)}>{formatted}</span>
}

export { Price }
