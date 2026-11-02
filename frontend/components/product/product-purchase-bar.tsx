"use client"

import Link from "next/link"
import { ShoppingBag } from "lucide-react"

import { AddToCartButton } from "@/components/cart/add-to-cart-button"
import { useCart } from "@/components/cart/cart-provider"
import { Price } from "@/components/common/price"
import { Button } from "@/components/ui/button"
import type { Product } from "@/types/product"

// Mobile-only bar pinned to the bottom of the product page so add-to-cart
// is always in thumb reach, however far down the description goes. The
// page adds bottom padding (pb-24 md:pb-0) so it never covers content.
// Phones don't use the cart drawer, so once the product is in the cart the
// bar offers the cart page instead.
function ProductPurchaseBar({ product }: { product: Product }) {
  const { getQuantity } = useCart()
  const inCart = getQuantity(product.id) > 0

  return (
    <div className="fixed inset-x-0 bottom-0 z-30 animate-in border-t bg-background/95 px-4 py-3 backdrop-blur duration-300 slide-in-from-bottom-4 md:hidden">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="line-clamp-1 text-sm font-medium">
            {product.name ?? "Unnamed product"}
          </p>
          <Price pence={product.price} className="text-sm font-semibold" />
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <AddToCartButton product={product} size="lg" />
          {inCart && (
            <Button
              size="lg"
              className="animate-in duration-200 fade-in-0 zoom-in-95"
              nativeButton={false}
              render={<Link href="/cart" />}
            >
              <ShoppingBag />
              Cart
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}

export { ProductPurchaseBar }
