import type { Metadata } from "next"
import Link from "next/link"
import { ArrowLeft, Hourglass, Mail, Phone, SearchX } from "lucide-react"

import { EmptyState } from "@/components/common/empty-state"
import { Price } from "@/components/common/price"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { AcceptCountdown } from "@/components/vendor/accept-countdown"
import { CaptureFailedAlert } from "@/components/vendor/capture-failed-alert"
import { LiveOrders } from "@/components/vendor/live-orders"
import { CollectByBadge } from "@/components/vendor/collect-by-badge"
import { FulfilmentLabel } from "@/components/vendor/fulfilment-label"
import { OrderItems } from "@/components/vendor/order-items"
import { OrderStatusActions } from "@/components/vendor/order-status-actions"
import { OrderStatusBadge } from "@/components/vendor/order-status-badge"
import { OrderTimeline } from "@/components/vendor/order-timeline"
import { PaymentCard } from "@/components/vendor/payment-card"
import { PageHeader } from "@/components/common/page-header"
import { formatDateTime } from "@/lib/format"
import { vendorAPI } from "@/lib/vendor/api"
import { handoverBlocked } from "@/lib/vendor/payment-status"
import { requireBranch } from "@/lib/vendor/session"

export const metadata: Metadata = { title: "Order" }

type Props = {
  params: Promise<{ branchId: string; orderId: string }>
  // ?code= from "Find by code": opens the hand-over with it filled in.
  searchParams: Promise<{ code?: string }>
}

export default async function OrderPage({ params, searchParams }: Props) {
  const { branchId, orderId } = await params
  const { code } = await searchParams
  const { session } = await requireBranch(branchId)
  const order = await vendorAPI.getOrder(branchId, orderId)

  const back = (
    <Button
      variant="ghost"
      size="sm"
      className="mb-4 -ml-2"
      nativeButton={false}
      render={<Link href={`/vendor/${branchId}/orders`} />}
    >
      <ArrowLeft />
      Orders
    </Button>
  )

  if (!order) {
    return (
      <>
        {back}
        <EmptyState
          icon={SearchX}
          title="Order not found"
          description="It may belong to another branch."
          className="rounded-xl border"
        />
      </>
    )
  }

  // The card couldn't be charged: hide the handover steps (the API refuses
  // them too). Ending the order, e.g. as a no-show, still works.
  const blocked = handoverBlocked(order.payment)
  const nextStatuses = blocked
    ? order.next_statuses.filter(
        (s) => s !== "completed" && s !== "out_for_delivery"
      )
    : order.next_statuses
  // Owners and managers. Until /me sends refund_orders, fall back to the
  // role; the API enforces it either way.
  const canRefund =
    session.permissions.refund_orders ?? session.user.role !== "staff"

  return (
    <>
      {back}
      {/* Shows the customer's substitution answers as they arrive. */}
      <LiveOrders branchId={branchId} fallbackMs={30_000} />
      <PageHeader
        title={
          <span className="flex flex-wrap items-center gap-3">
            {order.reference}
            <OrderStatusBadge
              status={order.status}
              className="h-6 px-2.5 text-sm"
            />
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
            Placed {formatDateTime(order.created_at)}
            {order.status === "pending" && (
              <AcceptCountdown deadline={order.accept_deadline} />
            )}
          </span>
        }
        actions={
          session.permissions.update_order_status && (
            <OrderStatusActions
              branchId={branchId}
              orderId={order.id}
              reference={order.reference}
              status={order.status}
              nextStatuses={nextStatuses}
              requiresCollectionCode={order.requires_collection_code}
              initialCode={code && /^\d{6}$/.test(code) ? code : undefined}
            />
          )
        }
      />

      {blocked && <CaptureFailedAlert amount={order.total} />}

      {order.awaiting_customer && (
        <div className="mb-4 flex animate-in items-start gap-3 rounded-xl border border-amber-500/40 bg-amber-500/5 p-4 text-sm fade-in-0">
          <Hourglass className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" />
          <p>
            Waiting for the customer to answer a substitute. You can mark the
            order ready once they do - if they don&apos;t answer in time, the
            item is removed automatically.
          </p>
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="flex min-w-0 flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle>Items</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              <OrderItems
                branchId={branchId}
                orderId={order.id}
                items={order.items}
                allowSubstitutions={order.allow_substitutions}
                editable={
                  session.permissions.update_order_status &&
                  (order.status === "accepted" || order.status === "picking")
                }
              />
              <Separator />
              <dl className="flex flex-col gap-1.5 text-sm">
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Subtotal</dt>
                  <dd>
                    <Price pence={order.subtotal} />
                  </dd>
                </div>
                {order.fulfilment.type === "delivery" && (
                  <div className="flex justify-between">
                    <dt className="text-muted-foreground">Delivery</dt>
                    <dd>
                      {order.delivery_fee === 0 ? (
                        "Free"
                      ) : (
                        <Price pence={order.delivery_fee} />
                      )}
                    </dd>
                  </div>
                )}
                <div className="flex justify-between text-base font-semibold">
                  <dt>Total</dt>
                  <dd>
                    <Price pence={order.total} />
                  </dd>
                </div>
              </dl>
              {!order.allow_substitutions && (
                <p className="text-xs text-muted-foreground">
                  The customer doesn&apos;t want substitutes - remove anything
                  that&apos;s out of stock.
                </p>
              )}
            </CardContent>
          </Card>

          {(order.notes || order.rejection_reason) && (
            <Card>
              <CardHeader>
                <CardTitle>Notes</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-3">
                {order.notes && (
                  <div>
                    <div className="text-xs font-medium text-muted-foreground">
                      From the customer
                    </div>
                    <p className="whitespace-pre-line">{order.notes}</p>
                  </div>
                )}
                {order.rejection_reason && (
                  <div>
                    <div className="text-xs font-medium text-muted-foreground">
                      Reason
                    </div>
                    <p>
                      {order.rejection_reason === "not_accepted_in_time"
                        ? "Not accepted in time, so it was cancelled automatically."
                        : order.rejection_reason}
                    </p>
                  </div>
                )}
              </CardContent>
            </Card>
          )}
        </div>

        <div className="flex min-w-0 flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle>Customer</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-2">
              <span className="font-medium">{order.customer.name}</span>
              <a
                href={`tel:${order.customer.phone.replace(/\s+/g, "")}`}
                className="inline-flex items-center gap-2 text-sm hover:underline"
              >
                <Phone className="size-4 text-muted-foreground" />
                {order.customer.phone}
              </a>
              <a
                href={`mailto:${order.customer.email}?subject=${encodeURIComponent(`Your order ${order.reference}`)}`}
                className="inline-flex min-w-0 items-center gap-2 text-sm hover:underline"
              >
                <Mail className="size-4 shrink-0 text-muted-foreground" />
                <span className="truncate">{order.customer.email}</span>
              </a>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Fulfilment</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-2 text-sm">
              <FulfilmentLabel
                type={order.fulfilment.type}
                date={order.fulfilment.date}
              />
              {order.status === "ready" && order.collect_by && (
                <CollectByBadge collectBy={order.collect_by} />
              )}
              {order.address && (
                <address className="text-muted-foreground not-italic">
                  {[
                    order.address.line1,
                    order.address.line2,
                    order.address.city,
                    order.address.postcode,
                  ]
                    .filter(Boolean)
                    .map((line) => (
                      <div key={line}>{line}</div>
                    ))}
                </address>
              )}
            </CardContent>
          </Card>

          {order.payment && (
            <PaymentCard
              payment={order.payment}
              branchId={branchId}
              orderId={order.id}
              reference={order.reference}
              canRefund={canRefund}
            />
          )}

          <Card>
            <CardHeader>
              <CardTitle>History</CardTitle>
            </CardHeader>
            <CardContent>
              <OrderTimeline
                createdAt={order.created_at}
                events={order.events}
              />
            </CardContent>
          </Card>
        </div>
      </div>
    </>
  )
}
