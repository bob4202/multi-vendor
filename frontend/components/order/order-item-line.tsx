import { ArrowRight, Hourglass } from "lucide-react"

import { Price } from "@/components/common/price"
import { OrderLineItem } from "@/components/order/order-line-item"
import type { OrderItem } from "@/types/order"

// One line of a placed order, showing what happened to it while picking:
// removed, swapped for a substitute, or waiting for the customer's answer.
function OrderItemLine({ item }: { item: OrderItem }) {
  const sub = item.substitution

  if (item.status === "removed") {
    return (
      <OrderLineItem
        name={item.name}
        quantity={item.quantity}
        unitPrice={item.unit_price}
        lineTotal={item.line_total}
        struck
        detail={
          <span className="text-muted-foreground">
            {sub?.status === "declined"
              ? "Removed - you declined the substitute"
              : sub?.status === "expired"
                ? "Removed - no answer in time"
                : "Out of stock - removed"}
          </span>
        }
      />
    )
  }

  if (item.status === "substituted" && sub) {
    return (
      <OrderLineItem
        name={item.name}
        quantity={item.quantity}
        unitPrice={item.unit_price}
        lineTotal={item.line_total}
        struck
        detail={
          <span className="flex flex-wrap items-center gap-x-1.5 font-medium text-foreground">
            <ArrowRight className="size-3.5 text-muted-foreground" />
            {sub.quantity} × {sub.name} ·{" "}
            <Price pence={sub.line_total} />
          </span>
        }
      />
    )
  }

  return (
    <OrderLineItem
      name={item.name}
      quantity={item.quantity}
      unitPrice={item.unit_price}
      lineTotal={item.line_total}
      detail={
        item.status === "awaiting_customer" && (
          <span className="flex items-center gap-1 font-medium text-amber-700 dark:text-amber-400">
            <Hourglass className="size-3.5" />
            Out of stock - waiting for your answer above
          </span>
        )
      }
    />
  )
}

export { OrderItemLine }
