"use client"

import Link from "next/link"
import { X } from "lucide-react"

import { useCart } from "@/components/cart/cart-provider"
import { QuantityStepper } from "@/components/cart/quantity-stepper"
import { Price } from "@/components/common/price"
import { ProductImage } from "@/components/product/product-image"
import { Button } from "@/components/ui/button"
import type { CartItem } from "@/lib/cart"
import { cn } from "@/lib/utils"

interface CartLineItemProps {
  item: CartItem
  // "compact" for the drawer (narrow, no description), "full" for /cart.
  variant?: "compact" | "full"
  // Called when the product link is followed - the drawer uses it to close
  // itself before navigating.
  onNavigate?: () => void
  className?: string
}

function CartLineItem({
  item,
  variant = "full",
  onNavigate,
  className,
}: CartLineItemProps) {
  const { setQuantity, removeItem } = useCart()
  const { product, quantity } = item
  const href = `/products/${product.id}`
  const compact = variant === "compact"
  const lineTotal = product.price === null ? null : product.price * quantity

  return (
    <li
      className={cn(
        "flex animate-in gap-3 py-4 duration-200 fade-in-0 slide-in-from-right-2",
        className
      )}
    >
      <Link
        href={href}
        onClick={onNavigate}
        className={cn(
          "shrink-0 overflow-hidden rounded-lg",
          compact ? "size-16" : "size-20 sm:size-24"
        )}
      >
        <ProductImage
          src={product.image_urls?.[0]}
          alt={product.name}
          iconClassName="size-6"
        />
      </Link>

      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <Link
              href={href}
              onClick={onNavigate}
              className="line-clamp-1 text-sm font-medium hover:underline"
            >
              {product.name ?? "Unnamed product"}
            </Link>
            {product.category_name && (
              <p className="text-xs text-muted-foreground">
                {product.category_name}
              </p>
            )}
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            className="-mt-0.5 -mr-1 text-muted-foreground"
            aria-label={`Remove ${product.name ?? "product"}`}
            onClick={() => removeItem(product.id)}
          >
            <X />
          </Button>
        </div>

        {!compact && product.description && (
          <p className="line-clamp-2 text-sm text-muted-foreground">
            {product.description}
          </p>
        )}

        <div className="mt-auto flex items-end justify-between gap-2 pt-1">
          <div className="flex flex-col gap-1">
            <QuantityStepper
              size="sm"
              removeAtMin
              value={quantity}
              onChange={(next) => setQuantity(product.id, next)}
            />
            {quantity > 1 && product.price !== null && (
              <span className="text-xs text-muted-foreground">
                <Price pence={product.price} /> each
              </span>
            )}
          </div>
          <Price
            pence={lineTotal}
            fallback="—"
            className="text-sm font-semibold"
          />
        </div>
      </div>
    </li>
  )
}

export { CartLineItem }
