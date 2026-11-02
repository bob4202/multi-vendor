"use server"

import { externalAPI } from "@/lib/external-api-service"
import { startPayment } from "@/lib/payment"
import type { Product } from "@/types/product"
import type {
  FulfilmentType,
  OrderIssue,
  PlaceOrderRequest,
} from "@/types/order"

// Fresh price/availability for the cart's products. Called when checkout
// opens; ids the branch no longer sells are simply missing from the result.
export async function refreshCart(ids: string[]): Promise<Product[]> {
  return externalAPI.getProductsByIds(ids.slice(0, 100))
}

export type PlaceOrderState =
  | { status: "idle" }
  // redirectUrl = the card payment page; without it (the page couldn't be
  // opened) the order page offers "Pay now" instead.
  | { status: "ok"; orderId: string; redirectUrl?: string }
  | { status: "error"; message: string; issues: OrderIssue[] }

function text(formData: FormData, key: string) {
  const value = formData.get(key)
  return typeof value === "string" ? value.trim() : ""
}

// Turns the checkout form into an order request, sends it to the API, then
// opens the card payment page for it. Returns (rather than redirects) so the
// client can remember the order before leaving for the payment page.
export async function placeOrder(
  _prev: PlaceOrderState,
  formData: FormData
): Promise<PlaceOrderState> {
  let items: PlaceOrderRequest["items"] = []
  try {
    items = JSON.parse(text(formData, "items"))
  } catch {
    return {
      status: "error",
      message: "Your basket couldn't be read.",
      issues: [],
    }
  }

  const type = text(formData, "fulfilment_type") as FulfilmentType
  const request: PlaceOrderRequest = {
    items,
    fulfilment: { type, date: text(formData, "fulfilment_date") },
    customer: {
      name: text(formData, "name"),
      email: text(formData, "email"),
      phone: text(formData, "phone"),
    },
    notes: text(formData, "notes"),
    expected_total: Number(text(formData, "expected_total")) || 0,
    // Only sent once the box is ticked; the API refuses the order without it.
    terms_version:
      formData.get("terms") === "on" ? text(formData, "terms_version") : "",
    allow_substitutions: formData.get("allow_substitutions") === "on",
  }
  if (type === "delivery") {
    request.address = {
      line1: text(formData, "line1"),
      line2: text(formData, "line2"),
      city: text(formData, "city"),
      postcode: text(formData, "postcode"),
    }
  }

  try {
    const result = await externalAPI.placeOrder(
      request,
      text(formData, "idempotency_key")
    )
    if (result.ok) {
      const payment = await startPayment(result.order.id)
      return {
        status: "ok",
        orderId: result.order.id,
        redirectUrl: payment.ok ? payment.redirectUrl : undefined,
      }
    }
    return {
      status: "error",
      message:
        result.status === 422
          ? "Please check the highlighted details."
          : result.error,
      issues: result.issues,
    }
  } catch {
    return {
      status: "error",
      message: "We couldn't reach the shop. Please try again.",
      issues: [],
    }
  }
}
