import type { Metadata } from "next"
import { notFound } from "next/navigation"

import { Container } from "@/components/layout/container"
import { OrderTracker } from "@/components/order/order-tracker"
import { externalAPI } from "@/lib/external-api-service"

export const metadata: Metadata = { title: "Your order" }

export default async function OrderPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>
  searchParams: Promise<{ payment?: string }>
}) {
  const { id } = await params
  // Set by the payment page on the way back: "return" after paying,
  // "cancelled" after backing out.
  const { payment } = await searchParams
  // A malformed id is a 400 from the API - treat it as not found too.
  const [order, pushKey] = await Promise.all([
    externalAPI.getOrder(id).catch(() => null),
    externalAPI.pushPublicKey(),
  ])
  if (!order) notFound()

  return (
    <Container className="py-10">
      <OrderTracker
        initialOrder={order}
        pushKey={pushKey}
        paymentReturn={payment === "return"}
      />
    </Container>
  )
}
