import "server-only"

import { headers } from "next/headers"

import { externalAPI } from "@/lib/external-api-service"

export type StartPaymentResult =
  { ok: true; redirectUrl: string } | { ok: false; error: string }

// This storefront's own address, from the request, so the API never needs
// to know where the storefront lives (one deployment per branch).
async function siteOrigin() {
  const h = await headers()
  const host = h.get("x-forwarded-host") ?? h.get("host") ?? "localhost:3000"
  const proto =
    h.get("x-forwarded-proto") ??
    (host.startsWith("localhost") || host.startsWith("127.") ? "http" : "https")
  return `${proto}://${host}`
}

// Gets the card payment page for an order. The provider sends the customer
// back to the order page, with ?payment= saying how they left.
export async function startPayment(
  orderId: string
): Promise<StartPaymentResult> {
  const orderUrl = `${await siteOrigin()}/orders/${encodeURIComponent(orderId)}`
  const res = await externalAPI
    .startPaymentSession(orderId, {
      success_url: `${orderUrl}?payment=return`,
      cancel_url: `${orderUrl}?payment=cancelled`,
    })
    .catch(() => null)
  if (res?.ok) return { ok: true, redirectUrl: res.data.redirect_url }
  if (res?.status === 409) {
    return {
      ok: false,
      error: "This order can't be paid for any more. Please refresh the page.",
    }
  }
  if (res?.status === 503) {
    return {
      ok: false,
      error:
        "This shop isn't taking card payments right now. Please call the shop.",
    }
  }
  return {
    ok: false,
    error: "We couldn't open the payment page. Please try again.",
  }
}
