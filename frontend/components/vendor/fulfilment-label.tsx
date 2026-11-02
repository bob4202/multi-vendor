import { Store, Truck } from "lucide-react"

import { formatDateLabel } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { FulfilmentType } from "@/types/vendor"

// "Collection · Wed 14 Oct" with a matching icon.
function FulfilmentLabel({
  type,
  date,
  className,
}: {
  type: FulfilmentType
  date: string
  className?: string
}) {
  const Icon = type === "delivery" ? Truck : Store
  return (
    <span className={cn("inline-flex items-center gap-1.5", className)}>
      <Icon className="size-3.5 shrink-0 text-muted-foreground" />
      <span>
        {type === "delivery" ? "Delivery" : "Collection"} ·{" "}
        {formatDateLabel(date)}
      </span>
    </span>
  )
}

export { FulfilmentLabel }
