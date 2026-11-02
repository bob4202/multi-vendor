"use client"

import { LayoutDashboard, Store } from "lucide-react"

import { AppShell } from "@/components/portal/app-shell"
import { adminLogoutAction } from "@/lib/admin/actions"
import type { PlatformAdmin } from "@/types/admin"

function AdminShell({
  admin,
  children,
}: {
  admin: PlatformAdmin
  children: React.ReactNode
}) {
  return (
    <AppShell
      homeHref="/admin"
      portalName="Admin"
      nav={[
        {
          href: "/admin",
          label: "Overview",
          icon: LayoutDashboard,
          exact: true,
        },
        { href: "/admin/vendors", label: "Vendors", icon: Store },
      ]}
      user={{ name: admin.name, email: admin.email, role: "Platform admin" }}
      logoutAction={adminLogoutAction}
    >
      {children}
    </AppShell>
  )
}

export { AdminShell }
