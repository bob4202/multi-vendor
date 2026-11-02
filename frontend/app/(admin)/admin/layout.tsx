import type { Metadata } from "next"
import { notFound } from "next/navigation"

import { Toaster } from "@/components/ui/sonner"
import { adminPortalEnabled } from "@/lib/admin/constants"

export const metadata: Metadata = {
  title: { default: "Admin", template: "%s | Admin" },
  robots: { index: false, follow: false },
}

// The platform admin side. proxy.ts already 404s it on deployments without
// ADMIN_PORTAL=enabled; this repeats the check in case the proxy matcher
// ever changes.
export default function AdminLayout({
  children,
}: {
  children: React.ReactNode
}) {
  if (!adminPortalEnabled()) notFound()
  return (
    <>
      {children}
      <Toaster position="top-center" />
    </>
  )
}
