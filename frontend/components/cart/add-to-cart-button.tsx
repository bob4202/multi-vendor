"use client"

import { ShoppingCart } from "lucide-react"

import { useCart } from "@/components/cart/cart-provider"
import { QuantityStepper } from "@/components/cart/quantity-stepper"
import type { Product } from "@/components/product/product-card"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

interface AddToCartButtonProps {
  product: Product
  size?: "sm" | "default" | "lg"
  // Open the cart drawer on the first add. Off on the grid (opening a drawer
  // per card would interrupt browsing - the header badge bumps instead), on
  // for the product page on desktop where adding is the end of the journey.
  // Phones don't use the drawer (see NavCart and ProductPurchaseBar).
  openCartOnAdd?: boolean
  className?: string
}

// Reused as-is on the product card (grid) and the product detail page: shows
// "Add to Cart" while the product isn't in the cart, then swaps to a
// quantity stepper once it is. `stopPropagation` on every control matters on
// the grid, where this sits on top of a card that's a full-bleed link to the
// product page.
function AddToCartButton({
  product,
  size = "default",
  openCartOnAdd = false,
  className,
}: AddToCartButtonProps) {
  const { getQuantity, addItem, setQuantity, setOpen } = useCart()
  const quantity = getQuantity(product.id)

  if (quantity === 0) {
    return (
      <Button
        type="button"
        size={size}
        className={cn("gap-1.5", className)}
        onClick={(event) => {
          event.stopPropagation()
          addItem(product, 1)
          if (openCartOnAdd) setOpen(true)
        }}
      >
        <ShoppingCart />
        Add to Cart
      </Button>
    )
  }

  return (
    <div
      className={cn("animate-in duration-150 fade-in-0 zoom-in-95", className)}
      onClick={(event) => event.stopPropagation()}
    >
      <QuantityStepper
        size={size === "sm" ? "sm" : "default"}
        value={quantity}
        onChange={(next) => setQuantity(product.id, next)}
        className={size === "lg" ? "h-9" : undefined}
      />
    </div>
  )
}

export { AddToCartButton }
