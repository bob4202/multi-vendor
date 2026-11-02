"use client"

import { useState, useTransition } from "react"
import { ArrowDown, Loader2, PackageX } from "lucide-react"
import { toast } from "sonner"

import { respondToSubstitutionAction } from "@/app/(storefront)/orders/actions"
import { Price } from "@/components/common/price"
import { Countdown } from "@/components/order/countdown"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import type { Order, OrderItem } from "@/types/order"

// The shop ran out of something and offers a substitute: the customer
// accepts it or has the item removed. One card per item waiting.
function SubstitutionPrompt({
  order,
  onUpdate,
}: {
  order: Order
  onUpdate: (order: Order) => void
}) {
  const waiting = order.items.filter(
    (i) =>
      i.status === "awaiting_customer" && i.substitution?.status === "proposed"
  )
  if (waiting.length === 0) return null

  return (
    <div className="flex flex-col gap-3">
      {waiting.map((item) => (
        <SubstitutionCard
          key={item.id}
          orderId={order.id}
          item={item}
          onUpdate={onUpdate}
        />
      ))}
    </div>
  )
}

function SubstitutionCard({
  orderId,
  item,
  onUpdate,
}: {
  orderId: string
  item: OrderItem
  onUpdate: (order: Order) => void
}) {
  const sub = item.substitution!
  const [pending, startTransition] = useTransition()
  const [choice, setChoice] = useState<boolean | null>(null)
  const normalPrice = sub.unit_price * sub.quantity
  const shopCovers = normalPrice > sub.line_total

  function answer(accept: boolean) {
    setChoice(accept)
    startTransition(async () => {
      const res = await respondToSubstitutionAction(orderId, sub.id, accept)
      if (res.order) onUpdate(res.order)
      if (res.ok) {
        toast.success(accept ? "Substitute accepted" : "Item removed")
      } else {
        toast.error(res.error)
      }
    })
  }

  return (
    <Card className="animate-in border-amber-500/50 bg-amber-500/5 duration-300 fade-in-0 slide-in-from-top-2">
      <CardContent className="flex flex-col gap-4">
        <div className="flex items-start gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-amber-500/15 text-amber-700 dark:text-amber-400">
            <PackageX className="size-5" />
          </span>
          <div className="flex min-w-0 flex-col gap-0.5">
            <h2 className="font-semibold">Something&apos;s out of stock</h2>
            <p className="text-sm text-muted-foreground">
              Answer within{" "}
              <Countdown
                to={sub.expires_at}
                expiredLabel="moments"
                className="font-semibold text-foreground"
              />
              , or it&apos;s removed from your order.
            </p>
          </div>
        </div>

        <div className="flex flex-col gap-1 rounded-lg bg-background p-3 text-sm">
          <div className="flex justify-between gap-3 text-muted-foreground">
            <span className="line-through">
              {item.quantity} × {item.name}
            </span>
            <Price pence={item.line_total} className="line-through" />
          </div>
          <ArrowDown className="my-0.5 size-4 text-muted-foreground" />
          <div className="flex justify-between gap-3 font-medium">
            <span>
              {sub.quantity} × {sub.name}
            </span>
            <Price pence={sub.line_total} />
          </div>
          {shopCovers && (
            <p className="text-xs text-muted-foreground">
              Normally <Price pence={normalPrice} /> - you only pay what you
              ordered.
            </p>
          )}
        </div>

        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <Button disabled={pending} onClick={() => answer(true)}>
            {pending && choice === true && <Loader2 className="animate-spin" />}
            Accept substitute
          </Button>
          <Button
            variant="outline"
            disabled={pending}
            onClick={() => answer(false)}
          >
            {pending && choice === false && (
              <Loader2 className="animate-spin" />
            )}
            Remove item
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

export { SubstitutionPrompt }
