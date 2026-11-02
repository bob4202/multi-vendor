import { CalendarClock, CircleAlert, Store } from "lucide-react"

import { formatTime } from "@/lib/format"
import type { BranchInfo } from "@/types/branch"

// Collection is same-day only: what today looks like at this shop.
function CollectionToday({
  today,
  address,
}: {
  today: BranchInfo["collection_today"]
  address?: string
}) {
  if (!today || !today.accepting) {
    return (
      <div className="flex items-start gap-3 rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm">
        <CircleAlert className="mt-0.5 size-4 shrink-0 text-destructive" />
        <p>
          {today
            ? `Last orders for collection today were at ${formatTime(today.last_order_at)}.`
            : "The shop isn't open for collection today."}{" "}
          Please come back tomorrow.
        </p>
      </div>
    )
  }
  return (
    <div className="flex flex-col gap-2 rounded-lg bg-muted p-3 text-sm">
      <p className="flex items-center gap-2 font-medium">
        <CalendarClock className="size-4 text-muted-foreground" />
        Collect today · order by {formatTime(today.last_order_at)}
      </p>
      {address && (
        <p className="flex items-center gap-2 text-muted-foreground">
          <Store className="size-4" />
          {address}
        </p>
      )}
      <p className="text-muted-foreground">
        We&apos;ll tell you when it&apos;s ready. Please collect within an hour
        of that, or by closing time ({formatTime(today.closes_at)}).
      </p>
    </div>
  )
}

export { CollectionToday }
