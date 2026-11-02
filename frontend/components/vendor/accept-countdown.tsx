import { Timer } from "lucide-react"

import { Countdown } from "@/components/order/countdown"
import { cn } from "@/lib/utils"

// "Accept within 4:32" on a new order. When the time runs out the API
// cancels it automatically (ORDER_ACCEPT_TIMEOUT_MINUTES), so staff can see
// how urgent each new order is.
function AcceptCountdown({
  deadline,
  className,
}: {
  deadline: string
  className?: string
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 text-xs font-medium text-amber-700 dark:text-amber-400",
        className
      )}
    >
      <Timer className="size-3.5" />
      Accept within <Countdown to={deadline} expiredLabel="time's up" />
    </span>
  )
}

export { AcceptCountdown }
