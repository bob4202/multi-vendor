"use client"

import { useEffect } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { ArrowLeft, ShoppingBag } from "lucide-react"

import { CartLineItem } from "@/components/cart/cart-line-item"
import { useCart } from "@/components/cart/cart-provider"
import { CartSuggestions } from "@/components/cart/cart-suggestions"
import { CartSummary } from "@/components/cart/cart-summary"
import { EmptyState } from "@/components/common/empty-state"
import { Price } from "@/components/common/price"
import { SwipeToConfirm } from "@/components/common/swipe-to-confirm"
import { Container } from "@/components/layout/container"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

export default function CartPage() {
  const { items, ready, totalItems, totalPrice } = useCart()
  const hasItems = ready && items.length > 0

  return (
    // Extra bottom padding on phones so the sticky summary bar never
    // covers the last of the page.
    <Container className={hasItems ? "pt-8 pb-44 md:pb-8" : "py-8"}>
      <h1 className="mb-6 text-2xl font-semibold">Your cart</h1>

      {!ready ? (
        <div className="grid grid-cols-1 gap-8 lg:grid-cols-[minmax(0,1fr)_360px]">
          <div className="flex flex-col gap-4">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="flex gap-3">
                <Skeleton className="size-20 sm:size-24" />
                <div className="flex flex-1 flex-col gap-2 py-1">
                  <Skeleton className="h-4 w-1/2" />
                  <Skeleton className="h-3 w-1/4" />
                </div>
              </div>
            ))}
          </div>
          <Skeleton className="h-56" />
        </div>
      ) : items.length === 0 ? (
        <EmptyState
          icon={ShoppingBag}
          title="Your cart is empty"
          description="Browse the shop and add something tasty."
          action={
            <Button nativeButton={false} render={<Link href="/shop" />}>
              Start shopping
            </Button>
          }
        />
      ) : (
        <div className="grid grid-cols-1 items-start gap-8 lg:grid-cols-[minmax(0,1fr)_360px]">
          <div className="min-w-0">
            <ul className="divide-y border-y">
              {items.map((item) => (
                <CartLineItem key={item.product.id} item={item} />
              ))}
            </ul>
            <Button
              variant="link"
              className="mt-4 px-0"
              nativeButton={false}
              render={<Link href="/shop" />}
            >
              <ArrowLeft />
              Continue shopping
            </Button>
            <CartSuggestions className="mt-8" />
          </div>

          {/* Phones get MobileCheckoutBar instead. */}
          <Card className="min-w-0 max-md:hidden lg:sticky lg:top-20">
            <CardContent>
              <h2 className="mb-4 font-semibold">Order summary</h2>
              <CartSummary>
                <Button
                  size="lg"
                  className="w-full"
                  nativeButton={false}
                  render={<Link href="/checkout" />}
                >
                  Checkout
                </Button>
              </CartSummary>
            </CardContent>
          </Card>

          <MobileCheckoutBar itemCount={totalItems} total={totalPrice} />
        </div>
      )}
    </Container>
  )
}

// Phones only: the order summary pinned to the bottom, with a swipe to
// check out, so it stays in thumb reach while scrolling the items and
// suggestions and a stray tap mid-scroll doesn't leave the page.
function MobileCheckoutBar({
  itemCount,
  total,
}: {
  itemCount: number
  total: number
}) {
  const router = useRouter()
  // A Link would prefetch on its own; the swipe navigates by hand.
  useEffect(() => router.prefetch("/checkout"), [router])

  return (
    <div className="fixed inset-x-0 bottom-0 z-30 flex animate-in flex-col gap-3 rounded-t-2xl border-t bg-background/95 px-4 pt-4 pb-[max(1rem,env(safe-area-inset-bottom))] shadow-[0_-8px_24px_-12px] shadow-black/20 backdrop-blur duration-300 slide-in-from-bottom-4 md:hidden">
      <div className="flex items-end justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium">
            Total{" "}
            <span className="font-normal text-muted-foreground">
              ({itemCount} {itemCount === 1 ? "item" : "items"})
            </span>
          </p>
          <p className="text-xs text-muted-foreground">
            Delivery calculated at checkout
          </p>
        </div>
        <Price pence={total} className="text-lg font-semibold" />
      </div>
      <SwipeToConfirm
        label="Swipe to checkout"
        onConfirm={() => router.push("/checkout")}
      />
    </div>
  )
}
