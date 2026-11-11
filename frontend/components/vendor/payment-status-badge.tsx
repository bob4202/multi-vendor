import { Badge } from "@/components/ui/badge"
import { PAYMENT_STATUS } from "@/lib/vendor/payment-status"
import { cn } from "@/lib/utils"
import type { PaymentStatus } from "@/types/vendor"

function PaymentStatusBadge({
  status,
  className,
}: {
  status: PaymentStatus
  className?: string
}) {
  const meta = PAYMENT_STATUS[status]
  return <Badge className={cn(meta.tone, className)}>{meta.label}</Badge>
}

export { PaymentStatusBadge }
