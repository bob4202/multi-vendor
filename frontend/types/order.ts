export type FulfilmentType = "collection" | "delivery"

export type OrderStatus =
  | "awaiting_payment" // placed, card not held yet; the shop can't see it
  | "pending"
  | "accepted"
  | "picking"
  | "ready"
  | "out_for_delivery"
  | "completed"
  | "rejected"
  | "cancelled"
  | "no_show"

// A line's state after the shop has picked it: still as ordered, waiting
// for the customer to answer a substitute, swapped, or taken off.
export type OrderItemStatus =
  | "ok"
  | "awaiting_customer"
  | "substituted"
  | "removed"

export interface Substitution {
  id: string
  product_id: string
  name: string
  quantity: number
  unit_price: number // the substitute's own price
  line_total: number // what the customer pays for it (never more than ordered)
  status: "proposed" | "accepted" | "declined" | "expired"
  expires_at: string // answer by this time, or the line is removed
}

export interface OrderItem {
  id: string
  product_id: string
  name: string
  unit_price: number // pence
  quantity: number
  line_total: number // pence, as ordered
  status: OrderItemStatus
  charged_total: number // what the line costs now
  substitution: Substitution | null
}

export type PaymentStatus =
  | "created" // the payment page is open
  | "authorised" // card held
  | "failed" // declined
  | "expired" // payment page timed out
  | "captured"
  | "voided" // hold released
  | "partially_refunded"
  | "refunded"
  | "capture_failed"

// The order's live payment, or else its newest attempt. Pence.
export interface OrderPayment {
  status: PaymentStatus
  amount_authorised: number
  amount_captured: number
  amount_refunded: number
}

export interface Order {
  id: string
  reference: string // "TH-10002"
  status: OrderStatus
  fulfilment: { type: FulfilmentType; date: string }
  customer: { name: string; email: string; phone: string }
  address: {
    line1: string
    line2: string | null
    city: string
    postcode: string
  } | null
  notes: string | null
  // "not_accepted_in_time" when the shop timed out, "payment_not_completed"
  // when the customer didn't pay by payment_deadline.
  rejection_reason: string | null
  items: OrderItem[]
  subtotal: number
  delivery_fee: number
  total: number
  created_at: string
  accept_deadline: string // ISO timestamp - the shop must accept before this
  accepted_at: string | null
  cancelled_at: string | null
  branch_name: string
  allow_substitutions: boolean
  awaiting_customer: boolean // a substitute is waiting for the customer's answer
  collection_code: string | null // shown from accepted to ready (collection only)
  ready_at: string | null
  collect_by: string | null // collect by then, or it becomes a no-show
  payment_deadline: string | null // awaiting_payment only: pay by then, or it's cancelled
  payment: OrderPayment | null // null before the first payment attempt
}

// One row of "My orders" (GET /v1/store/me/orders).
export interface CustomerOrderSummary {
  id: string
  reference: string
  status: OrderStatus
  branch_name: string
  fulfilment: { type: FulfilmentType; date: string }
  item_count: number
  total: number
  created_at: string
  needs_action: boolean // payment not finished, or a substitute is waiting for an answer
}

// One reason an order was refused (422). `field` matches a checkout input,
// `product_id` a basket line; neither = applies to the whole order.
export interface OrderIssue {
  code:
    | "required"
    | "invalid"
    | "invalid_quantity"
    | "unavailable"
    | "fulfilment_unavailable"
    | "invalid_date"
    | "outside_delivery_area"
    | "below_min_order"
    | "price_changed"
    | (string & {})
  field?: string
  product_id?: string
  message: string
}

export interface PlaceOrderRequest {
  items: { product_id: string; quantity: number }[]
  fulfilment: { type: FulfilmentType; date: string }
  customer: { name: string; email: string; phone: string }
  address?: { line1: string; line2: string; city: string; postcode: string }
  notes: string
  expected_total: number
  terms_version: string
  allow_substitutions: boolean
}
