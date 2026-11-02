import type { Metadata } from "next"
import Link from "next/link"
import { redirect } from "next/navigation"
import { LogOut, Receipt } from "lucide-react"

import { CustomerOrderList } from "@/components/account/customer-order-list"
import { ProfileForm } from "@/components/account/profile-form"
import { EmptyState } from "@/components/common/empty-state"
import { PageHeader } from "@/components/common/page-header"
import { SimplePagination } from "@/components/common/simple-pagination"
import { Container } from "@/components/layout/container"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { customerLogoutAction } from "@/lib/customer/actions"
import { CUSTOMER_LOGIN_PATH } from "@/lib/customer/constants"
import { getCustomer } from "@/lib/customer/session"
import { externalAPI } from "@/lib/external-api-service"

export const metadata: Metadata = { title: "Your account" }

export default async function AccountPage({
  searchParams,
}: {
  searchParams: Promise<{ page?: string }>
}) {
  const customer = await getCustomer()
  if (!customer) redirect(`${CUSTOMER_LOGIN_PATH}?next=/account`)

  const page = Math.max(1, Number((await searchParams).page) || 1)
  const orders = await externalAPI.myOrders(page).catch(() => null)

  return (
    <Container className="py-8">
      <PageHeader
        title="Your account"
        description={customer.email}
        actions={
          <form action={customerLogoutAction}>
            <Button type="submit" variant="outline">
              <LogOut />
              Sign out
            </Button>
          </form>
        }
      />

      <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-[minmax(0,1fr)_340px]">
        <section className="flex min-w-0 flex-col gap-3">
          <h2 className="font-semibold">Your orders</h2>
          {!orders?.ok ? (
            <p className="text-sm text-muted-foreground">
              Your orders couldn&apos;t be loaded. Please try again.
            </p>
          ) : orders.data.data.length === 0 ? (
            <EmptyState
              icon={Receipt}
              title="No orders yet"
              description="Orders you place while signed in, or with this email, show up here."
              action={
                <Button nativeButton={false} render={<Link href="/shop" />}>
                  Start shopping
                </Button>
              }
              className="rounded-xl border"
            />
          ) : (
            <>
              <CustomerOrderList orders={orders.data.data} />
              <SimplePagination
                page={orders.data.page}
                totalPages={orders.data.total_pages}
                hrefForPage={(p) => (p > 1 ? `/account?page=${p}` : "/account")}
              />
            </>
          )}
        </section>

        <Card className="min-w-0">
          <CardHeader>
            <CardTitle>Your details</CardTitle>
          </CardHeader>
          <CardContent>
            <ProfileForm customer={customer} />
          </CardContent>
        </Card>
      </div>
    </Container>
  )
}
