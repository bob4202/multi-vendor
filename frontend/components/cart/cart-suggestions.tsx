"use client"

import { useEffect, useRef, useState } from "react"
import Link from "next/link"

import { getCartSuggestions } from "@/app/(storefront)/cart/actions"
import { AddToCartButton } from "@/components/cart/add-to-cart-button"
import { useCart } from "@/components/cart/cart-provider"
import { Price } from "@/components/common/price"
import { ProductImage } from "@/components/product/product-image"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import type { Product } from "@/types/product"

// How many cards are on screen at once; the rest of the pool waits.
const VISIBLE_COUNT = 6

interface CartSuggestionsProps {
  title?: string
  className?: string
}

// Horizontally scrolling "add to your order" row. The pool is fetched once
// when the cart is ready; a product that lands in the cart (from here or
// anywhere else) drops out and the next one from the pool joins at the
// end, so there's always something new to add.
function CartSuggestions({
  title = "Add to your order",
  className,
}: CartSuggestionsProps) {
  const { items, ready, getQuantity } = useCart()
  const [pool, setPool] = useState<Product[] | null>(null)
  const fetched = useRef(false)

  useEffect(() => {
    if (!ready || fetched.current || items.length === 0) return
    fetched.current = true
    getCartSuggestions(
      items.map((i) => i.product.id),
      items.flatMap((i) =>
        i.product.category_id ? [i.product.category_id] : []
      )
    )
      .then(setPool)
      .catch(() => setPool([])) // nice to have - just hide the row
  }, [ready, items])

  const products =
    pool && pool.filter((p) => getQuantity(p.id) === 0).slice(0, VISIBLE_COUNT)

  if (!ready || items.length === 0 || products?.length === 0) return null

  const cardClass = "w-36 sm:w-40"

  return (
    <section className={cn("flex min-w-0 flex-col gap-3", className)}>
      <h2 className="text-lg font-semibold">{title}</h2>
      {/* Bleeds to the screen edge below lg so the row reads as scrollable. */}
      <ul className="-mx-4 flex snap-x scroll-px-4 [scrollbar-width:none] gap-3 overflow-x-auto px-4 pb-1 sm:-mx-6 sm:scroll-px-6 sm:px-6 lg:mx-0 lg:scroll-px-0 lg:px-0">
        {products === null
          ? Array.from({ length: 4 }).map((_, i) => (
              <li
                key={i}
                className={cn("flex shrink-0 flex-col gap-2", cardClass)}
              >
                <Skeleton className="aspect-4/3 w-full rounded-lg" />
                <Skeleton className="h-3.5 w-3/4" />
                <Skeleton className="h-7 w-full" />
              </li>
            ))
          : products.map((product) => (
              <SuggestionCard
                key={product.id}
                product={product}
                className={cardClass}
              />
            ))}
      </ul>
    </section>
  )
}

function SuggestionCard({
  product,
  className,
}: {
  product: Product
  className?: string
}) {
  const href = `/products/${product.id}`

  return (
    <li
      className={cn(
        "flex shrink-0 animate-in snap-start flex-col gap-2 duration-300 fade-in-0",
        className
      )}
    >
      <Link
        href={href}
        className="aspect-4/3 overflow-hidden rounded-lg bg-muted"
      >
        <ProductImage
          src={product.image_urls?.[0]}
          alt={product.name}
          iconClassName="size-6"
        />
      </Link>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <Link
          href={href}
          className="line-clamp-2 text-sm leading-snug font-medium hover:underline"
        >
          {product.name ?? "Unnamed product"}
        </Link>
        <Price
          pence={product.price}
          className="text-sm text-muted-foreground"
        />
      </div>
      <AddToCartButton product={product} size="sm" className="w-full" />
    </li>
  )
}

export { CartSuggestions }
