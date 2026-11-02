"use client"

import { useCart } from "@/components/cart/cart-provider"
import { PriceBreakdown } from "@/components/order/price-breakdown"
import { cn } from "@/lib/utils"

interface CartSummaryProps {
  // Checkout / view-cart buttons - the drawer and the cart page want
  // different ones.
  children?: React.ReactNode
  className?: string
}

function CartSummary({ children, className }: CartSummaryProps) {
  const { totalItems, totalPrice } = useCart()

  return (
    <div className={cn("flex flex-col gap-3", className)}>
      <PriceBreakdown
        subtotal={totalPrice}
        itemCount={totalItems}
        delivery="Calculated at checkout"
        total={totalPrice}
      />
      {children}
    </div>
  )
}

export { CartSummary }
