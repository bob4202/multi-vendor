import { CircleAlert } from "lucide-react"

import { Price } from "@/components/common/price"
import { cn } from "@/lib/utils"

interface OrderLineItemProps {
  name: string
  quantity: number
  unitPrice: number | null
  lineTotal: number | null
  // Shown under the line in red - e.g. "No longer available".
  warning?: string
  // Shown under the line - e.g. what replaced it.
  detail?: React.ReactNode
  // Crossed out without a warning (e.g. replaced by a substitute).
  struck?: boolean
  className?: string
}

// A read-only basket line ("2 × Lamb Chops ... £28.00"). Used where the
// quantity can't be changed: checkout summary, order page, shop screen.
function OrderLineItem({
  name,
  quantity,
  unitPrice,
  lineTotal,
  warning,
  detail,
  struck,
  className,
}: OrderLineItemProps) {
  const crossed = !!warning || struck
  return (
    <li className={cn("flex flex-col gap-1 py-2.5 text-sm", className)}>
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 gap-2">
          <span className="shrink-0 font-medium tabular-nums">
            {quantity} ×
          </span>
          <span className={cn("min-w-0", crossed && "line-through opacity-60")}>
            {name}
          </span>
        </div>
        <Price
          pence={lineTotal}
          fallback="—"
          className={cn(
            "shrink-0 font-medium",
            crossed && "opacity-60",
            struck && "line-through"
          )}
        />
      </div>
      {quantity > 1 && unitPrice !== null && !crossed && (
        <span className="pl-7 text-xs text-muted-foreground">
          <Price pence={unitPrice} /> each
        </span>
      )}
      {warning && (
        <span className="flex items-center gap-1 pl-7 text-xs text-destructive">
          <CircleAlert className="size-3.5" />
          {warning}
        </span>
      )}
      {detail && <div className="pl-7 text-xs">{detail}</div>}
    </li>
  )
}

export { OrderLineItem }
