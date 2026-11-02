import type { Metadata } from "next"
import { Store } from "lucide-react"

import { CheckoutForm } from "@/components/checkout/checkout-form"
import { EmptyState } from "@/components/common/empty-state"
import { Container } from "@/components/layout/container"
import { getCustomer } from "@/lib/customer/session"
import { externalAPI } from "@/lib/external-api-service"

export const metadata: Metadata = { title: "Checkout" }

export default async function CheckoutPage() {
  const [branch, customer] = await Promise.all([
    externalAPI.getBranch(),
    getCustomer(),
  ])

  return (
    <Container className="py-8">
      <h1 className="mb-6 text-2xl font-semibold">Checkout</h1>
      {branch?.payments.enabled ? (
        <CheckoutForm branch={branch} customer={customer} />
      ) : branch ? (
        // Card is the only way to pay, so no payment account = no orders.
        <EmptyState
          icon={Store}
          title="This shop isn't taking online orders right now"
          description="Please call in or visit the shop. Your basket is saved for later."
        />
      ) : (
        <EmptyState
          icon={Store}
          title="Online ordering isn't available yet"
          description="This shop hasn't been set up for online orders. Please call in or visit the shop."
        />
      )}
    </Container>
  )
}
