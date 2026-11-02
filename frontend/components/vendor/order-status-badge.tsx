import { Badge } from "@/components/ui/badge"
import { ORDER_STATUS } from "@/lib/vendor/order-status"
import { cn } from "@/lib/utils"
import type { OrderStatus } from "@/types/vendor"

function OrderStatusBadge({
  status,
  className,
}: {
  status: OrderStatus
  className?: string
}) {
  const meta = ORDER_STATUS[status]
  return <Badge className={cn(meta.tone, className)}>{meta.label}</Badge>
}

export { OrderStatusBadge }
