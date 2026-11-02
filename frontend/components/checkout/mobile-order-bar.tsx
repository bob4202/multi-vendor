import { Price } from "@/components/common/price"

interface MobileOrderBarProps {
  subtotal: number
  // A fee for delivery (0 = free); undefined for collection.
  delivery?: number
  total: number
  // Terms, errors and the swipe to place the order.
  children: React.ReactNode
}

// Phones only: the money and the "place order" step pinned to the bottom
// of checkout, so the total and the terms are always in view while filling
// in the form. The full summary card still lists the items above.
function MobileOrderBar({
  subtotal,
  delivery,
  total,
  children,
}: MobileOrderBarProps) {
  return (
    <div className="fixed inset-x-0 bottom-0 z-30 flex animate-in flex-col gap-3 rounded-t-2xl border-t bg-background/95 px-4 pt-4 pb-[max(1rem,env(safe-area-inset-bottom))] shadow-[0_-8px_24px_-12px] shadow-black/20 backdrop-blur duration-300 slide-in-from-bottom-4 md:hidden">
      <div className="flex items-end justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium">Total</p>
          <p className="text-xs text-muted-foreground">
            {delivery === undefined ? (
              <>Collection · pay by card</>
            ) : delivery === 0 ? (
              <>
                <Price pence={subtotal} /> + free delivery
              </>
            ) : (
              <>
                <Price pence={subtotal} /> + <Price pence={delivery} /> delivery
              </>
            )}
          </p>
        </div>
        <Price pence={total} className="text-lg font-semibold" />
      </div>
      {children}
    </div>
  )
}

export { MobileOrderBar }
