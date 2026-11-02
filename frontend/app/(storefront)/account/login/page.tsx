import type { Metadata } from "next"
import { redirect } from "next/navigation"

import { CustomerLoginForm } from "@/components/account/customer-login-form"
import { Container } from "@/components/layout/container"
import { Card, CardContent } from "@/components/ui/card"
import { safeCustomerRedirect } from "@/lib/customer/constants"
import { getCustomer } from "@/lib/customer/session"

export const metadata: Metadata = { title: "Sign in" }

export default async function CustomerLoginPage({
  searchParams,
}: {
  searchParams: Promise<{ next?: string }>
}) {
  const { next } = await searchParams
  if (await getCustomer()) redirect(safeCustomerRedirect(next))

  return (
    <Container className="flex justify-center py-12">
      <div className="flex w-full max-w-sm flex-col gap-6">
        <div className="flex flex-col gap-1 text-center">
          <h1 className="text-2xl font-semibold">Sign in</h1>
          <p className="text-sm text-muted-foreground">
            See your orders and check out faster next time.
          </p>
        </div>
        <Card>
          <CardContent>
            <CustomerLoginForm next={next} />
          </CardContent>
        </Card>
      </div>
    </Container>
  )
}
