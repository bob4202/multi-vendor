import Link from "next/link"
import { BellRing, ChevronRight } from "lucide-react"

import { Price } from "@/components/common/price"
import { OrderStatusBadge } from "@/components/order/order-status-badge"
import { formatDateTime } from "@/lib/format"
import type { CustomerOrderSummary } from "@/types/order"

// "My orders": one tappable row per order, newest first. Orders waiting on
// the customer (a substitute to approve) say so.
function CustomerOrderList({ orders }: { orders: CustomerOrderSummary[] }) {
  return (
    <ul className="flex flex-col divide-y rounded-xl border">
      {orders.map((o) => (
        <li key={o.id}>
          <Link
            href={`/orders/${o.id}`}
            className="flex items-center gap-3 p-4 transition-colors hover:bg-muted/50"
          >
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium tabular-nums">{o.reference}</span>
                <OrderStatusBadge
                  order={{ status: o.status, rejection_reason: null }}
                />
              </div>
              <span className="text-xs text-muted-foreground">
                {formatDateTime(o.created_at)} · {o.branch_name} ·{" "}
                {o.item_count} item{o.item_count === 1 ? "" : "s"}
              </span>
              {o.needs_action && (
                <span className="flex items-center gap-1 text-xs font-medium text-amber-700 dark:text-amber-400">
                  <BellRing className="size-3.5" />
                  The shop needs your answer about a substitute
                </span>
              )}
            </div>
            <Price pence={o.total} className="font-medium" />
            <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
          </Link>
        </li>
      ))}
    </ul>
  )
}

export { CustomerOrderList }
