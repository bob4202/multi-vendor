import type { OrderStatus } from "@/types/vendor"

// Presentation for order statuses. Which status can follow which is decided
// by the API (OrderDetail.next_statuses); this only says how to show them.

interface StatusMeta {
  label: string
  // Verb for the button that moves an order *into* this status.
  action: string
  // Tailwind classes for the status badge.
  tone: string
  // Moving into this status ends the order unhappily, so ask first.
  destructive?: boolean
}

export const ORDER_STATUS: Record<OrderStatus, StatusMeta> = {
  pending: {
    label: "New",
    action: "Mark as new",
    tone: "bg-amber-500/15 text-amber-700 dark:text-amber-400",
  },
  accepted: {
    label: "Accepted",
    action: "Accept",
    tone: "bg-sky-500/15 text-sky-700 dark:text-sky-400",
  },
  picking: {
    label: "Picking",
    action: "Start picking",
    tone: "bg-indigo-500/15 text-indigo-700 dark:text-indigo-400",
  },
  ready: {
    label: "Ready",
    action: "Mark ready",
    tone: "bg-violet-500/15 text-violet-700 dark:text-violet-400",
  },
  out_for_delivery: {
    label: "Out for delivery",
    action: "Send out for delivery",
    tone: "bg-cyan-500/15 text-cyan-700 dark:text-cyan-400",
  },
  completed: {
    label: "Completed",
    action: "Complete",
    tone: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400",
  },
  rejected: {
    label: "Rejected",
    action: "Reject",
    tone: "bg-destructive/10 text-destructive",
    destructive: true,
  },
  cancelled: {
    label: "Cancelled",
    action: "Cancel order",
    tone: "bg-muted text-muted-foreground",
    destructive: true,
  },
  no_show: {
    label: "No-show",
    action: "Mark no-show",
    tone: "bg-muted text-muted-foreground",
    destructive: true,
  },
}

// A reason is required to reject, optional (but offered) for the other
// unhappy endings.
export function reasonRequired(status: OrderStatus) {
  return status === "rejected"
}
