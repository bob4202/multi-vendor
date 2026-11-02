"use client"

import { useState, useTransition } from "react"
import { ArrowRight, Hourglass, Loader2, PackageX } from "lucide-react"
import { toast } from "sonner"

import { Price } from "@/components/common/price"
import { Countdown } from "@/components/order/countdown"
import { Button } from "@/components/ui/button"
import { SubstituteDialog } from "@/components/vendor/substitute-dialog"
import { withdrawSubstitutionAction } from "@/lib/vendor/actions"
import { cn } from "@/lib/utils"
import type { OrderItem } from "@/types/vendor"

// The order's lines as the shop picks them. While the order is accepted or
// being picked, each line can be marked out of stock (removed, or swapped
// for a substitute the customer approves).
function OrderItems({
  branchId,
  orderId,
  items,
  editable,
  allowSubstitutions,
}: {
  branchId: string
  orderId: string
  items: OrderItem[]
  editable: boolean
  allowSubstitutions: boolean
}) {
  const [editing, setEditing] = useState<OrderItem | null>(null)

  return (
    <>
      <ul className="flex flex-col gap-3">
        {items.map((item) => (
          <OrderItemRow
            key={item.id}
            branchId={branchId}
            orderId={orderId}
            item={item}
            editable={editable}
            onOutOfStock={() => setEditing(item)}
          />
        ))}
      </ul>
      {editing && (
        <SubstituteDialog
          key={editing.id}
          branchId={branchId}
          orderId={orderId}
          item={editing}
          allowSubstitutions={allowSubstitutions}
          open
          onOpenChange={(open) => !open && setEditing(null)}
        />
      )}
    </>
  )
}

function OrderItemRow({
  branchId,
  orderId,
  item,
  editable,
  onOutOfStock,
}: {
  branchId: string
  orderId: string
  item: OrderItem
  editable: boolean
  onOutOfStock: () => void
}) {
  const [pending, startTransition] = useTransition()
  const sub = item.substitution
  const gone = item.status === "removed" || item.status === "substituted"

  function withdraw() {
    startTransition(async () => {
      const res = await withdrawSubstitutionAction(branchId, orderId, item.id)
      if (res.ok) toast.success("Offer withdrawn")
      else toast.error(res.error)
    })
  }

  return (
    <li className="flex flex-col gap-2">
      <div className="flex items-start gap-3">
        <span
          className={cn(
            "flex h-7 min-w-9 items-center justify-center rounded-md bg-muted px-1.5 text-sm font-semibold tabular-nums",
            gone && "opacity-50"
          )}
        >
          {item.quantity}×
        </span>
        <div className="flex min-w-0 flex-1 flex-col">
          <span className={cn("font-medium", gone && "line-through opacity-60")}>
            {item.name}
          </span>
          <span className="text-xs text-muted-foreground">
            <Price pence={item.unit_price} /> each
            {item.plu && ` · PLU ${item.plu}`}
          </span>
        </div>
        <div className="flex shrink-0 flex-col items-end gap-1">
          <Price
            pence={item.charged_total}
            className={cn("font-medium", item.status === "removed" && "opacity-60")}
          />
          {item.charged_total !== item.line_total && (
            <Price
              pence={item.line_total}
              className="text-xs text-muted-foreground line-through"
            />
          )}
        </div>
      </div>

      {item.status === "substituted" && sub && (
        <p className="flex flex-wrap items-center gap-1.5 pl-12 text-sm font-medium">
          <ArrowRight className="size-4 text-muted-foreground" />
          {sub.quantity} × {sub.name}
          <span className="font-normal text-muted-foreground">
            (accepted by the customer)
          </span>
        </p>
      )}

      {item.status === "removed" && (
        <p className="pl-12 text-xs text-muted-foreground">
          {sub?.status === "declined"
            ? "Removed - the customer declined the substitute"
            : sub?.status === "expired"
              ? "Removed - the customer didn't answer in time"
              : "Out of stock - removed"}
        </p>
      )}

      {item.status === "awaiting_customer" && sub && (
        <div className="ml-12 flex flex-col gap-2 rounded-lg border border-amber-500/40 bg-amber-500/5 p-3 text-sm sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-col gap-0.5">
            <span className="flex items-center gap-1.5 font-medium">
              <Hourglass className="size-4 text-amber-600 dark:text-amber-400" />
              Offered {sub.quantity} × {sub.name} ·{" "}
              <Price pence={sub.line_total} />
            </span>
            <span className="text-xs text-muted-foreground">
              Waiting for the customer · removed in{" "}
              <Countdown to={sub.expires_at} expiredLabel="moments" />
            </span>
          </div>
          {editable && (
            <Button
              size="sm"
              variant="ghost"
              disabled={pending}
              onClick={withdraw}
              className="self-start sm:self-auto"
            >
              {pending && <Loader2 className="animate-spin" />}
              Withdraw offer
            </Button>
          )}
        </div>
      )}

      {editable && item.status === "ok" && (
        <Button
          size="sm"
          variant="outline"
          className="ml-12 self-start"
          onClick={onOutOfStock}
        >
          <PackageX />
          Out of stock
        </Button>
      )}
    </li>
  )
}

export { OrderItems }
