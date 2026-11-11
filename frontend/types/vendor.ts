// Shapes returned by the vendor portal API (/v1/vendor/*). Money is in
// pence; dates are "YYYY-MM-DD", timestamps ISO 8601.

import type {
  OrderItemStatus,
  OrderPayment,
  PaymentStatus,
  Substitution,
} from "@/types/order"

export type { ActionResult, ApiIssue, Paginated } from "@/types/api"
export type { OrderItemStatus, OrderPayment, PaymentStatus, Substitution }

export type VendorRole = "owner" | "manager" | "staff"

export interface VendorUser {
  id: string
  name: string
  email: string
  role: VendorRole
}

export interface VendorBranch {
  id: string
  name: string
  is_active: boolean
}

// What the signed-in user may do. Comes from the API so the UI never has to
// mirror the role table - the API still enforces it either way.
export interface VendorPermissions {
  update_order_status: boolean
  edit_store_info: boolean
  trigger_sync: boolean
  // Owners and managers. Optional until /me sends it (see order page).
  refund_orders?: boolean
}

export interface VendorSession {
  user: VendorUser
  vendor: { id: string; name: string }
  branches: VendorBranch[]
  permissions: VendorPermissions
}

export type OrderStatus =
  | "pending"
  | "accepted"
  | "picking"
  | "ready"
  | "out_for_delivery"
  | "completed"
  | "rejected"
  | "cancelled"
  | "no_show"

export type FulfilmentType = "collection" | "delivery"

export interface OrderSummary {
  id: string
  reference: string
  status: OrderStatus
  fulfilment: { type: FulfilmentType; date: string }
  customer: { name: string; email: string; phone: string }
  item_count: number
  total: number
  created_at: string
  // A pending order is cancelled automatically if not accepted by then.
  accept_deadline: string
  awaiting_customer: boolean // a substitute is waiting for the customer
  collect_by: string | null // ready collection orders: collect by then
  // Optional until the list endpoint sends it; the detail always does.
  payment?: OrderPayment | null
}

export interface OrderItem {
  id: string
  product_id: string
  name: string
  plu: string | null
  unit_price: number
  quantity: number
  line_total: number
  status: OrderItemStatus
  charged_total: number
  substitution: Substitution | null
}

export interface OrderEvent {
  // payment: card held, capture failed, refund requested (status unchanged)
  kind: "status" | "substitution" | "removed" | "code_override" | "payment"
  from_status: OrderStatus | null
  to_status: OrderStatus
  note: string | null
  actor_name: string | null
  created_at: string
}

export interface OrderDetail extends Omit<OrderSummary, "item_count"> {
  next_statuses: OrderStatus[]
  address: {
    line1: string | null
    line2: string | null
    city: string | null
    postcode: string | null
  } | null
  notes: string | null
  rejection_reason: string | null
  items: OrderItem[]
  subtotal: number
  delivery_fee: number
  events: OrderEvent[]
  updated_at: string
  accepted_at: string | null
  cancelled_at: string | null
  allow_substitutions: boolean
  ready_at: string | null
  // Handing over needs the customer's code (staff never see it).
  requires_collection_code: boolean
  payment: OrderPayment | null // null for orders from before card payments
}

export interface OpeningHour {
  day: number // ISO: 1 = Monday ... 7 = Sunday
  open: string // "HH:MM"
  close: string
}

export interface StoreInfo {
  phone: string
  email: string | null
  address: {
    line1: string | null
    line2: string | null
    city: string | null
    postcode: string | null
  }
  fulfilment: {
    collection: { enabled: boolean; last_order_minutes: number }
    delivery: {
      enabled: boolean
      fee: number
      free_over: number | null
      min_order: number
      postcode_prefixes: string[]
    }
  }
  opening_hours: OpeningHour[]
}

export interface Closure {
  id: number
  date: string
  closed: boolean
  open: string | null
  close: string | null
  note: string | null
}

export interface SyncState {
  resource: string
  last_run_at: string | null
  last_run_status: "success" | "error" | null
  last_run_error: string | null
  last_run_records: number | null
}
