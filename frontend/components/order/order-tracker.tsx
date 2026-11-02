"use client"

import { useEffect, useState } from "react"
import Link from "next/link"
import {
  CircleCheck,
  CircleX,
  Clock,
  CreditCard,
  Hourglass,
  PackageCheck,
  PackageX,
  Truck,
} from "lucide-react"

import { getOrderStatus } from "@/app/(storefront)/orders/actions"
import { CollectionCodeCard } from "@/components/order/collection-code-card"
import { Countdown } from "@/components/order/countdown"
import { OrderItemLine } from "@/components/order/order-item-line"
import { OrderStatusBadge } from "@/components/order/order-status-badge"
import { PriceBreakdown } from "@/components/order/price-breakdown"
import { PushOptIn } from "@/components/order/push-opt-in"
import { SubstitutionPrompt } from "@/components/order/substitution-prompt"
import { PaymentRetry } from "@/components/order/payment-retry"
import { useLive } from "@/components/order/use-live"
import { useCart } from "@/components/cart/cart-provider"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { takePayingOrder } from "@/lib/cart"
import { formatDateLabel, formatPrice, formatTime } from "@/lib/format"
import { isFinished, timedOut, unpaid } from "@/lib/order-status"
import { cn } from "@/lib/utils"
import type { Order } from "@/types/order"

// The customer's live view of an order. Changes arrive over a live stream
// (falling back to polling), so the page follows the order from "waiting
// for the shop" to collected without a reload.
function OrderTracker({
  initialOrder,
  pushKey,
  paymentReturn,
}: {
  initialOrder: Order
  pushKey: string
  // Back from the payment page after paying: show "confirming" until the
  // provider tells the API the card is held.
  paymentReturn: boolean
}) {
  const [order, setOrder] = useState(initialOrder)
  const finished = isFinished(order.status)
  const awaitingPayment = order.status === "awaiting_payment"
  const { clear } = useCart()

  // A declined card doesn't change the order itself, so the live stream
  // stays quiet: poll while we wait to hear how the payment went.
  useEffect(() => {
    if (!awaitingPayment || !paymentReturn) return
    const timer = window.setInterval(async () => {
      const latest = await getOrderStatus(order.id).catch(() => null)
      if (latest) setOrder(latest)
    }, 3000)
    return () => window.clearInterval(timer)
  }, [awaitingPayment, paymentReturn, order.id])

  // Paid: the basket this tab paid with can go now.
  useEffect(() => {
    if (awaitingPayment || unpaid(order)) return
    if (takePayingOrder(order.id)) clear()
  }, [awaitingPayment, order, clear])

  useLive<Order>({
    url: `/orders/${order.id}/events`,
    eventName: "order",
    onEvent: setOrder,
    poll: async () => {
      const latest = await getOrderStatus(order.id).catch(() => null)
      if (latest) setOrder(latest)
    },
    pollMs: order.status === "pending" ? 4000 : 15000,
    active: !finished,
  })

  const isDelivery = order.fulfilment.type === "delivery"
  const orderedTotal = order.items.reduce((sum, i) => sum + i.line_total, 0)
  const totalChanged = orderedTotal !== order.subtotal

  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-6">
      <StatusHero order={order} paymentReturn={paymentReturn} />

      <SubstitutionPrompt order={order} onUpdate={setOrder} />

      {order.collection_code && (
        <CollectionCodeCard
          code={order.collection_code}
          collectBy={order.collect_by}
          ready={order.status === "ready"}
        />
      )}

      {!finished && !awaitingPayment && pushKey && (
        <PushOptIn orderId={order.id} publicKey={pushKey} />
      )}

      <Card>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <p className="text-xs text-muted-foreground">Order</p>
              <p className="font-semibold tabular-nums">{order.reference}</p>
            </div>
            <OrderStatusBadge order={order} />
          </div>

          <dl className="grid gap-3 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground">
                {isDelivery ? "Delivery" : "Collection"}
              </dt>
              <dd className="font-medium">
                {formatDateLabel(order.fulfilment.date)}
                {!isDelivery && order.branch_name && ` · ${order.branch_name}`}
              </dd>
            </div>
            {order.address && (
              <div>
                <dt className="text-muted-foreground">Deliver to</dt>
                <dd className="font-medium">
                  {order.address.line1}, {order.address.postcode}
                </dd>
              </div>
            )}
          </dl>

          <ul className="divide-y border-y">
            {order.items.map((item) => (
              <OrderItemLine key={item.id ?? item.product_id} item={item} />
            ))}
          </ul>

          <PriceBreakdown
            subtotal={order.subtotal}
            delivery={isDelivery ? order.delivery_fee : undefined}
            total={order.total}
          />
          <p className="text-xs text-muted-foreground">
            {totalChanged &&
              "Your total has been updated for the changes above. "}
            {paymentNote(order)}
          </p>
        </CardContent>
      </Card>
    </div>
  )
}

// One line under the totals: where the money is.
function paymentNote(order: Order) {
  const p = order.payment
  if (order.status === "awaiting_payment") {
    return "Pay by card to send your order to the shop."
  }
  if (!p) return null
  switch (p.status) {
    case "authorised":
      return `${formatPrice(p.amount_authorised)} is held on your card. You're only charged the final total when your order is ready.`
    case "captured":
    case "partially_refunded":
    case "refunded":
      return p.amount_refunded > 0
        ? `Charged ${formatPrice(p.amount_captured)} to your card, ${formatPrice(p.amount_refunded)} refunded.`
        : `Charged ${formatPrice(p.amount_captured)} to your card.`
    case "voided":
      return "The hold on your card has been released."
    case "capture_failed":
      return "There was a problem taking the payment from your card. The shop will be in touch."
    default:
      return null
  }
}

interface Hero {
  icon: typeof Hourglass
  tone: string
  title: string
  body: React.ReactNode
  pulse?: boolean
  backToShop?: boolean
  action?: React.ReactNode
}

const TONE = {
  waiting: "text-amber-600 dark:text-amber-400 bg-amber-500/10",
  good: "text-emerald-600 dark:text-emerald-400 bg-emerald-500/10",
  bad: "text-destructive bg-destructive/10",
  neutral: "text-muted-foreground bg-muted",
}

// The big icon + headline at the top: what's happening right now, in words.
function heroFor(order: Order, paymentReturn: boolean): Hero {
  const isDelivery = order.fulfilment.type === "delivery"
  const when = formatDateLabel(order.fulfilment.date)
  const collectBy = order.collect_by ? formatTime(order.collect_by) : null

  if (order.status === "awaiting_payment") {
    const payment = order.payment?.status
    // "authorised" while still awaiting_payment = the API is moving the
    // order on right now.
    if (paymentReturn && (payment === "created" || payment === "authorised")) {
      return {
        icon: CreditCard,
        tone: TONE.waiting,
        pulse: true,
        title: "Confirming your payment",
        body: "This usually takes a few seconds. You can keep this page open - it updates by itself.",
      }
    }
    const declined = payment === "failed"
    return {
      icon: CreditCard,
      tone: declined ? TONE.bad : TONE.waiting,
      title: declined ? "Your card was declined" : "Payment not finished",
      body: (
        <>
          {declined
            ? "Nothing was taken from your card. Try again, or use a different card. "
            : "Your order goes to the shop once it's paid. "}
          {order.payment_deadline && (
            <>
              You have{" "}
              <Countdown
                to={order.payment_deadline}
                expiredLabel="moments"
                className="font-semibold text-foreground"
              />{" "}
              left to pay.
            </>
          )}
        </>
      ),
      action: (
        <PaymentRetry
          orderId={order.id}
          label={declined ? "Try again" : `Pay ${formatPrice(order.total)}`}
        />
      ),
    }
  }
  if (unpaid(order)) {
    return {
      icon: Clock,
      tone: TONE.bad,
      backToShop: true,
      title: "Payment wasn't completed in time",
      body: "Your order was cancelled and nothing was taken from your card. If you'd still like it, please place a new order.",
    }
  }
  if (order.status === "pending") {
    return {
      icon: Hourglass,
      tone: TONE.waiting,
      pulse: true,
      title: "Waiting for the shop to accept",
      body: (
        <>
          The shop has{" "}
          <Countdown
            to={order.accept_deadline}
            expiredLabel="moments"
            className="font-semibold text-foreground"
          />{" "}
          left to confirm. You can keep this page open - it updates by itself.
        </>
      ),
    }
  }
  if (timedOut(order)) {
    return {
      icon: Clock,
      tone: TONE.bad,
      backToShop: true,
      title: "The shop didn't respond in time",
      body: "Your order was cancelled automatically and you haven't been charged. Please try again or call the shop.",
    }
  }
  if (order.status === "rejected" || order.status === "cancelled") {
    return {
      icon: CircleX,
      tone: TONE.bad,
      backToShop: true,
      title:
        order.status === "rejected"
          ? "The shop couldn't take your order"
          : "Your order was cancelled",
      body: order.rejection_reason
        ? `Reason: ${order.rejection_reason}. You haven't been charged.`
        : "You haven't been charged. Please try again or call the shop.",
    }
  }
  if (order.status === "no_show") {
    return {
      icon: PackageX,
      tone: TONE.bad,
      title: "Not collected in time",
      body: `Your order was closed because it wasn't collected${collectBy ? ` by ${collectBy}` : ""}. As set out in the terms, uncollected orders aren't refunded.`,
    }
  }
  if (order.status === "completed") {
    return {
      icon: PackageCheck,
      tone: TONE.good,
      title: isDelivery ? "Delivered" : "Collected - thank you!",
      body: "Enjoy your order. We hope to see you again soon.",
    }
  }
  if (order.awaiting_customer) {
    return {
      icon: PackageX,
      tone: TONE.waiting,
      pulse: true,
      title: "The shop needs your answer",
      body: "An item is out of stock. Please choose below so the shop can finish your order.",
    }
  }
  if (order.status === "ready") {
    return {
      icon: PackageCheck,
      tone: TONE.good,
      title: isDelivery ? "Ready for delivery" : "Ready to collect",
      body: isDelivery
        ? `Your order is packed and will be delivered on ${when}.`
        : `Head to ${order.branch_name || "the shop"}${collectBy ? ` and collect it by ${collectBy}` : ""}. Show your collection code at the counter.`,
    }
  }
  if (order.status === "out_for_delivery") {
    return {
      icon: Truck,
      tone: TONE.good,
      title: "On its way",
      body: "Your order is out for delivery.",
    }
  }
  return {
    icon: CircleCheck,
    tone: TONE.good,
    title: order.status === "picking" ? "Being prepared" : "Order confirmed!",
    body: isDelivery
      ? `The shop is preparing your order for delivery on ${when}.`
      : "The shop is preparing your order. We'll let you know as soon as it's ready to collect.",
  }
}

function StatusHero({
  order,
  paymentReturn,
}: {
  order: Order
  paymentReturn: boolean
}) {
  const hero = heroFor(order, paymentReturn)
  const Icon = hero.icon

  return (
    <div
      key={`${order.status}-${order.awaiting_customer}-${order.payment?.status}`}
      className="flex animate-in flex-col items-center gap-3 text-center duration-300 fade-in-0 zoom-in-95"
    >
      <div
        className={cn(
          "flex size-14 items-center justify-center rounded-full",
          hero.tone
        )}
      >
        <Icon className={cn("size-7", hero.pulse && "animate-pulse")} />
      </div>
      <h1 className="text-2xl font-semibold">{hero.title}</h1>
      <p className="max-w-md text-sm text-muted-foreground">{hero.body}</p>
      {hero.action}
      {hero.backToShop && (
        <Button nativeButton={false} render={<Link href="/shop" />}>
          Back to the shop
        </Button>
      )}
    </div>
  )
}

export { OrderTracker }
