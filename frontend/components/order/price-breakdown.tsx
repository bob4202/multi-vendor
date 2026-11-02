import { Price } from "@/components/common/price"
import { cn } from "@/lib/utils"

interface PriceBreakdownProps {
  subtotal: number
  itemCount?: number
  // number = a real fee (0 shows "Free"); string = a note such as
  // "Calculated at checkout"; undefined = no delivery row (collection).
  delivery?: number | string
  total: number
  className?: string
}

// Subtotal / delivery / total rows. Presentational only, so the cart, the
// checkout, the order page and the shop's screen all show money the same way.
function PriceBreakdown({
  subtotal,
  itemCount,
  delivery,
  total,
  className,
}: PriceBreakdownProps) {
  return (
    <div className={cn("flex flex-col gap-3 text-sm", className)}>
      <div className="flex flex-col gap-1.5">
        <div className="flex justify-between text-muted-foreground">
          <span>
            Subtotal
            {itemCount !== undefined &&
              ` (${itemCount} ${itemCount === 1 ? "item" : "items"})`}
          </span>
          <Price pence={subtotal} />
        </div>
        {delivery !== undefined && (
          <div className="flex justify-between text-muted-foreground">
            <span>Delivery</span>
            {typeof delivery === "string" ? (
              <span>{delivery}</span>
            ) : delivery === 0 ? (
              <span className="font-medium text-emerald-600 dark:text-emerald-400">
                Free
              </span>
            ) : (
              <Price pence={delivery} />
            )}
          </div>
        )}
      </div>
      <div className="flex justify-between border-t pt-3 text-base font-semibold">
        <span>Total</span>
        <Price pence={total} />
      </div>
    </div>
  )
}

export { PriceBreakdown }
