"use server"

import {
  externalAPI,
  type PushSubscriptionJSON,
} from "@/lib/external-api-service"
import { startPayment, type StartPaymentResult } from "@/lib/payment"
import type { Order } from "@/types/order"

// Polled by the order page while the shop decides.
export async function getOrderStatus(id: string): Promise<Order | null> {
  return externalAPI.getOrder(id)
}

// "Pay now" / "Try again" on an order awaiting payment.
export async function startPaymentAction(
  orderId: string
): Promise<StartPaymentResult> {
  return startPayment(orderId)
}

export type RespondResult =
  | { ok: true; order: Order }
  | { ok: false; error: string; order: Order | null }

// The customer's answer to a substitute the shop offered.
export async function respondToSubstitutionAction(
  orderId: string,
  substitutionId: string,
  accept: boolean
): Promise<RespondResult> {
  const res = await externalAPI
    .respondToSubstitution(orderId, substitutionId, accept)
    .catch(() => null)
  if (res?.ok) return { ok: true, order: res.data }
  // Already answered, expired or withdrawn: show the order as it is now.
  const latest = await externalAPI.getOrder(orderId).catch(() => null)
  return {
    ok: false,
    error:
      res?.status === 409
        ? "This substitute is no longer waiting for an answer."
        : "We couldn't send your answer. Please try again.",
    order: latest,
  }
}

// Links this browser's push subscription to the order (and to the
// signed-in customer, if any), so the shop's updates reach it.
export async function savePushSubscriptionAction(
  subscription: PushSubscriptionJSON,
  orderId: string
): Promise<boolean> {
  const res = await externalAPI
    .savePushSubscription(subscription, orderId)
    .catch(() => null)
  return !!res?.ok
}

export async function deletePushSubscriptionAction(endpoint: string) {
  await externalAPI.deletePushSubscription(endpoint).catch(() => null)
}
