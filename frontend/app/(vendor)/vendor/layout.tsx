import type { Metadata } from "next"

import { Toaster } from "@/components/ui/sonner"

// The vendor portal opts out of the storefront chrome (header, cart,
// footer) entirely; each section brings its own shell.
export const metadata: Metadata = {
  title: { default: "Vendor portal", template: "%s | Vendor portal" },
  robots: { index: false, follow: false },
}

export default function VendorLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <>
      {children}
      <Toaster position="top-center" />
    </>
  )
}
