"use client"

import Link from "next/link"
import { ShoppingBag } from "lucide-react"

import { useCart } from "@/components/cart/cart-provider"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

// Phones go straight to /cart - the drawer would be full-screen there
// anyway, and the page has room for suggestions and a sticky checkout bar.
// From md up it opens the drawer. Switched with CSS, so server and client
// render the same markup.
export function NavCart() {
  const { totalItems, setOpen } = useCart()
  const label = `Open cart, ${totalItems} items`
  const badge = totalItems > 0 && (
    <Badge
      // Re-keyed on the count so every add/remove replays the pop -
      // the grid's only feedback that an add landed.
      key={totalItems}
      className="absolute -top-1 -right-1 h-5 min-w-5 animate-in justify-center rounded-full bg-red-600 px-1.5 text-xs text-white tabular-nums duration-200 zoom-in-50"
    >
      {totalItems > 99 ? "99+" : totalItems}
    </Badge>
  )

  return (
    <>
      <Button
        variant="ghost"
        size="icon"
        className="relative md:hidden"
        aria-label={label}
        nativeButton={false}
        render={<Link href="/cart" />}
      >
        <ShoppingBag />
        {badge}
      </Button>
      <Button
        variant="ghost"
        size="icon"
        className="relative hidden md:inline-flex"
        aria-label={label}
        onClick={() => setOpen(true)}
      >
        <ShoppingBag />
        {badge}
      </Button>
    </>
  )
}
