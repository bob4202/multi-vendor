import Link from "next/link"
import { ChevronRight, Hourglass } from "lucide-react"

import { Price } from "@/components/common/price"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { AcceptCountdown } from "@/components/vendor/accept-countdown"
import { CollectByBadge } from "@/components/vendor/collect-by-badge"
import { FulfilmentLabel } from "@/components/vendor/fulfilment-label"
import { OrderStatusBadge } from "@/components/vendor/order-status-badge"
import { formatDateTime } from "@/lib/format"
import type { OrderSummary } from "@/types/vendor"

function itemsLabel(count: number) {
  return `${count} item${count === 1 ? "" : "s"}`
}

// Orders as a table from `md`, as tappable cards below it.
function OrderList({
  orders,
  branchId,
}: {
  orders: OrderSummary[]
  branchId: string
}) {
  const href = (o: OrderSummary) => `/vendor/${branchId}/orders/${o.id}`

  return (
    <>
      <ul className="flex flex-col gap-2 md:hidden">
        {orders.map((o) => (
          <li key={o.id}>
            <Link
              href={href(o)}
              className="flex flex-col gap-2 rounded-xl border p-4 transition-colors hover:bg-muted/50"
            >
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium">{o.reference}</span>
                <OrderStatusBadge status={o.status} />
              </div>
              <OrderFlags order={o} />
              <div className="flex items-center justify-between gap-2 text-sm">
                <span className="truncate">{o.customer.name}</span>
                <Price pence={o.total} className="font-medium" />
              </div>
              <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                <FulfilmentLabel
                  type={o.fulfilment.type}
                  date={o.fulfilment.date}
                />
                <span>{itemsLabel(o.item_count)}</span>
              </div>
            </Link>
          </li>
        ))}
      </ul>

      <div className="hidden overflow-hidden rounded-xl border md:block">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Order</TableHead>
              <TableHead>Customer</TableHead>
              <TableHead>Fulfilment</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Total</TableHead>
              <TableHead className="w-8" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {orders.map((o) => (
              <TableRow key={o.id} className="relative">
                <TableCell>
                  {/* The link's ::after covers the whole row, so any click
                      on it opens the order. */}
                  <Link
                    href={href(o)}
                    className="font-medium after:absolute after:inset-0"
                  >
                    {o.reference}
                  </Link>
                  <div className="text-xs text-muted-foreground">
                    {formatDateTime(o.created_at)}
                  </div>
                </TableCell>
                <TableCell>
                  <div>{o.customer.name}</div>
                  <div className="text-xs text-muted-foreground">
                    {itemsLabel(o.item_count)}
                  </div>
                </TableCell>
                <TableCell>
                  <FulfilmentLabel
                    type={o.fulfilment.type}
                    date={o.fulfilment.date}
                  />
                </TableCell>
                <TableCell>
                  <div className="flex flex-col items-start gap-1">
                    <OrderStatusBadge status={o.status} />
                    <OrderFlags order={o} />
                  </div>
                </TableCell>
                <TableCell className="text-right">
                  <Price pence={o.total} className="font-medium" />
                </TableCell>
                <TableCell>
                  <ChevronRight className="size-4 text-muted-foreground" />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </>
  )
}

export { OrderList }

// What needs attention on an order: time left to accept, a substitute the
// customer hasn't answered, or when a ready order must be collected.
function OrderFlags({ order }: { order: OrderSummary }) {
  return (
    <>
      {order.status === "pending" && (
        <AcceptCountdown deadline={order.accept_deadline} />
      )}
      {order.awaiting_customer && (
        <span className="inline-flex items-center gap-1 text-xs font-medium text-amber-700 dark:text-amber-400">
          <Hourglass className="size-3.5" />
          Waiting for customer
        </span>
      )}
      {order.status === "ready" && order.collect_by && (
        <CollectByBadge collectBy={order.collect_by} />
      )}
    </>
  )
}
