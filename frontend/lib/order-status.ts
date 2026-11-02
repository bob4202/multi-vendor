import type { Order, OrderStatus } from "@/types/order"

type Tone = "waiting" | "good" | "bad" | "neutral"

// One place for how each status reads and looks, shared by the customer's
// order page and the shop's /vendor screen.
export const ORDER_STATUS: Record<OrderStatus, { label: string; tone: Tone }> =
  {
    awaiting_payment: { label: "Payment not finished", tone: "waiting" },
    pending: { label: "Waiting for shop", tone: "waiting" },
    accepted: { label: "Accepted", tone: "good" },
    picking: { label: "Being prepared", tone: "good" },
    ready: { label: "Ready", tone: "good" },
    out_for_delivery: { label: "Out for delivery", tone: "good" },
    completed: { label: "Completed", tone: "neutral" },
    rejected: { label: "Declined", tone: "bad" },
    cancelled: { label: "Cancelled", tone: "bad" },
    no_show: { label: "Not collected", tone: "bad" },
  }

export function timedOut(order: Pick<Order, "status" | "rejection_reason">) {
  return (
    order.status === "cancelled" &&
    order.rejection_reason === "not_accepted_in_time"
  )
}

// Cancelled because the customer didn't finish paying in time.
export function unpaid(order: Pick<Order, "status" | "rejection_reason">) {
  return (
    order.status === "cancelled" &&
    order.rejection_reason === "payment_not_completed"
  )
}

// No more changes will happen to an order in one of these.
export function isFinished(status: OrderStatus) {
  return (
    status === "completed" ||
    status === "rejected" ||
    status === "cancelled" ||
    status === "no_show"
  )
}
