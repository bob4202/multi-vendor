import type { Metadata } from "next"
import { Geist, Geist_Mono } from "next/font/google"

import "./globals.css"
import { CartProvider } from "@/components/cart/cart-provider"
import { ThemeProvider } from "@/components/theme-provider"
import { config } from "@/lib/config"
import { cn } from "@/lib/utils"

const geist = Geist({ subsets: ["latin"], variable: "--font-sans" })

const fontMono = Geist_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
})

export const metadata: Metadata = {
  title: {
    default: "Grocery Shop",
    template: "%s | Grocery Shop",
  },
  description: "Fresh groceries from your local shop.",
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={cn(
        "antialiased",
        fontMono.variable,
        "font-sans",
        geist.variable
      )}
    >
      <body>
        <ThemeProvider>
          <CartProvider branchId={config.api.branchId ?? "default"}>
            {children}
          </CartProvider>
        </ThemeProvider>
      </body>
    </html>
  )
}
