"use client"

import { ClipboardList, Package, Store } from "lucide-react"

import { AppShell } from "@/components/portal/app-shell"
import { BranchSwitcher } from "@/components/vendor/branch-switcher"
import { logoutAction } from "@/lib/vendor/actions"
import { vendorPortalUser } from "@/lib/vendor/portal-user"
import type { VendorSession } from "@/types/vendor"

// The vendor portal's shell for one branch: its sections, plus a branch
// switcher for owners running several.
function VendorShell({
  session,
  branchId,
  children,
}: {
  session: VendorSession
  branchId: string
  children: React.ReactNode
}) {
  const base = `/vendor/${branchId}`
  return (
    <AppShell
      homeHref="/vendor"
      portalName="Vendor portal"
      nav={[
        { href: `${base}/orders`, label: "Orders", icon: ClipboardList },
        { href: `${base}/store`, label: "Store details", icon: Store },
        { href: `${base}/products`, label: "Products", icon: Package },
      ]}
      sidebarTop={
        <BranchSwitcher branches={session.branches} currentId={branchId} />
      }
      user={vendorPortalUser(session)}
      logoutAction={logoutAction}
    >
      {children}
    </AppShell>
  )
}

export { VendorShell }
