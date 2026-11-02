"use client"

import { useState } from "react"
import Link from "next/link"
import { Menu } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet"
import { ThemeToggle } from "@/components/layout/theme-toggle"
import { SidebarNav, type NavItem } from "@/components/portal/sidebar-nav"
import { UserMenu, type PortalUser } from "@/components/portal/user-menu"

// Chrome for a signed-in portal (vendor or admin): a fixed sidebar from
// `lg`, a top bar with a slide-out menu below that. `sidebarTop` sits above
// the nav in both (e.g. the vendor's branch switcher).
function AppShell({
  homeHref,
  portalName,
  nav,
  sidebarTop,
  user,
  logoutAction,
  children,
}: {
  homeHref: string
  portalName: string
  nav: NavItem[]
  sidebarTop?: React.ReactNode
  user: PortalUser
  logoutAction: () => Promise<void>
  children: React.ReactNode
}) {
  const [menuOpen, setMenuOpen] = useState(false)

  const brand = (
    <Link href={homeHref} className="flex flex-col leading-tight">
      <span className="font-bold tracking-tight">Grocery Shop</span>
      <span className="text-xs text-muted-foreground">{portalName}</span>
    </Link>
  )

  return (
    <div className="min-h-svh lg:grid lg:grid-cols-[16rem_minmax(0,1fr)]">
      <aside className="sticky top-0 hidden h-svh flex-col gap-6 border-r bg-muted/30 p-4 lg:flex">
        <div className="flex items-center justify-between">
          {brand}
          <ThemeToggle />
        </div>
        {sidebarTop}
        <SidebarNav items={nav} />
        <div className="mt-auto border-t pt-3">
          <UserMenu user={user} logoutAction={logoutAction} showName />
        </div>
      </aside>

      <div className="flex min-w-0 flex-col">
        <header className="sticky top-0 z-40 flex h-14 items-center justify-between gap-2 border-b bg-background/80 px-4 backdrop-blur supports-backdrop-filter:bg-background/60 lg:hidden">
          <Sheet open={menuOpen} onOpenChange={setMenuOpen}>
            <SheetTrigger
              render={
                <Button variant="ghost" size="icon" aria-label="Open menu" />
              }
            >
              <Menu />
            </SheetTrigger>
            <SheetContent side="left">
              <SheetHeader>
                <SheetTitle>Menu</SheetTitle>
              </SheetHeader>
              <div className="flex flex-col gap-4 px-4">
                {sidebarTop}
                <SidebarNav items={nav} onNavigate={() => setMenuOpen(false)} />
              </div>
            </SheetContent>
          </Sheet>
          {brand}
          <div className="flex items-center gap-1">
            <ThemeToggle />
            <UserMenu user={user} logoutAction={logoutAction} />
          </div>
        </header>

        <main className="flex-1 px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
          {children}
        </main>
      </div>
    </div>
  )
}

export { AppShell }
