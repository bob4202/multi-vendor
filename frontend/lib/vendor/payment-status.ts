import type { OrderPayment, PaymentStatus } from "@/types/vendor"

// How the vendor portal shows a card payment. The API decides what can
// happen to it; this only says how to show it.

interface PaymentMeta {
  label: string
  // Tailwind classes for the badge, same scheme as ORDER_STATUS.
  tone: string
}

export const PAYMENT_STATUS: Record<PaymentStatus, PaymentMeta> = {
  created: { label: "Not paid", tone: "bg-muted text-muted-foreground" },
  failed: { label: "Not paid", tone: "bg-muted text-muted-foreground" },
  expired: { label: "Not paid", tone: "bg-muted text-muted-foreground" },
  authorised: {
    label: "Card held",
    tone: "bg-sky-500/15 text-sky-700 dark:text-sky-400",
  },
  captured: {
    label: "Paid",
    tone: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400",
  },
  voided: { label: "Hold released", tone: "bg-muted text-muted-foreground" },
  partially_refunded: {
    label: "Part refunded",
    tone: "bg-amber-500/15 text-amber-700 dark:text-amber-400",
  },
  refunded: { label: "Refunded", tone: "bg-muted text-muted-foreground" },
  capture_failed: {
    label: "Not charged",
    tone: "bg-destructive/10 text-destructive",
  },
}

// Money taken and not yet given back. The API also counts refunds that are
// still queued, so it can refuse a little more than this shows.
export function refundableAmount(payment: OrderPayment | null | undefined) {
  if (
    !payment ||
    (payment.status !== "captured" && payment.status !== "partially_refunded")
  ) {
    return 0
  }
  return Math.max(0, payment.amount_captured - payment.amount_refunded)
}

// The order can't leave the shop: the API answers 409 to these.
export function handoverBlocked(payment: OrderPayment | null | undefined) {
  return payment?.status === "capture_failed"
}
