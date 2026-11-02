import type { ReactNode } from "react"

import { CartDrawer } from "@/components/cart/cart-drawer"
import { SiteHeader } from "@/components/layout/site-header"
import { SiteFooter } from "@/components/layout/site-footer"

// Chrome shared by every storefront page (header/nav/footer). Root layout
// stays minimal (fonts, theme) so a future route group - an admin section,
// say - can opt out of this shell entirely rather than fight it.
export default function StorefrontLayout({
  children,
}: {
  children: ReactNode
}) {
  return (
    <div className="flex min-h-svh flex-col">
      <SiteHeader />
      <main className="flex-1">{children}</main>
      <SiteFooter />
      <CartDrawer />
    </div>
  )
}
