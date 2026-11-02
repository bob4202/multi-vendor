import { Badge } from "@/components/ui/badge"
import { ORDER_STATUS, timedOut } from "@/lib/order-status"
import { cn } from "@/lib/utils"
import type { Order } from "@/types/order"

const TONE_CLASS = {
  waiting:
    "border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300",
  good: "border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  bad: "border-destructive/40 bg-destructive/10 text-destructive",
  neutral: "",
} as const

function OrderStatusBadge({
  order,
  className,
}: {
  order: Pick<Order, "status" | "rejection_reason">
  className?: string
}) {
  const { label, tone } = ORDER_STATUS[order.status]
  return (
    <Badge variant="outline" className={cn(TONE_CLASS[tone], className)}>
      {tone === "waiting" && (
        <span className="size-1.5 animate-pulse rounded-full bg-current" />
      )}
      {timedOut(order) ? "Timed out" : label}
    </Badge>
  )
}

export { OrderStatusBadge }
