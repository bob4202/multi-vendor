"use client"

import Link from "next/link"
import { ShoppingBag } from "lucide-react"

import { CartLineItem } from "@/components/cart/cart-line-item"
import { useCart } from "@/components/cart/cart-provider"
import { CartSummary } from "@/components/cart/cart-summary"
import { EmptyState } from "@/components/common/empty-state"
import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"

// Mounted once in the storefront layout; opened from the header cart button
// or by AddToCartButton (openCartOnAdd) via the cart context.
function CartDrawer() {
  const { items, totalItems, isOpen, setOpen } = useCart()
  const close = () => setOpen(false)

  return (
    <Sheet open={isOpen} onOpenChange={setOpen}>
      <SheetContent
        side="right"
        className="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-md"
      >
        <SheetHeader className="border-b">
          <SheetTitle>Your cart</SheetTitle>
          <SheetDescription>
            {totalItems === 0
              ? "Nothing here yet."
              : `${totalItems} ${totalItems === 1 ? "item" : "items"}`}
          </SheetDescription>
        </SheetHeader>

        {items.length === 0 ? (
          <EmptyState
            icon={ShoppingBag}
            title="Your cart is empty"
            description="Browse the shop and add something tasty."
            className="my-auto"
            action={
              <Button
                nativeButton={false}
                render={<Link href="/shop" onClick={close} />}
              >
                Start shopping
              </Button>
            }
          />
        ) : (
          <>
            <ScrollArea className="min-h-0 flex-1">
              <ul className="divide-y px-4">
                {items.map((item) => (
                  <CartLineItem
                    key={item.product.id}
                    item={item}
                    variant="compact"
                    onNavigate={close}
                  />
                ))}
              </ul>
            </ScrollArea>

            <SheetFooter className="border-t">
              <CartSummary>
                <div className="grid grid-cols-2 gap-2">
                  <Button
                    variant="outline"
                    nativeButton={false}
                    render={<Link href="/cart" onClick={close} />}
                  >
                    View cart
                  </Button>
                  <Button
                    nativeButton={false}
                    render={<Link href="/checkout" onClick={close} />}
                  >
                    Checkout
                  </Button>
                </div>
              </CartSummary>
            </SheetFooter>
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}

export { CartDrawer }
