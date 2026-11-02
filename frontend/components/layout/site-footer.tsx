import Link from "next/link"

import { Container } from "@/components/layout/container"
import { NAV_LINKS } from "@/lib/nav-links"

function SiteFooter() {
  const year = new Date().getFullYear()

  return (
    <footer className="border-t">
      <Container className="grid gap-8 py-10 sm:grid-cols-2 lg:grid-cols-4">
        <div className="flex flex-col gap-2">
          <span className="text-lg font-bold tracking-tight">Grocery Shop</span>
          <p className="text-sm text-muted-foreground">
            Fresh groceries from your local shop.
          </p>
        </div>

        <div className="flex flex-col gap-2">
          <h3 className="text-sm font-medium">Shop</h3>
          <nav className="flex flex-col gap-2">
            {NAV_LINKS.map((link) => (
              <Link
                key={link.href}
                href={link.href}
                className="text-sm text-muted-foreground hover:text-foreground"
              >
                {link.label}
              </Link>
            ))}
          </nav>
        </div>

        <div className="flex flex-col gap-2">
          <h3 className="text-sm font-medium">Support</h3>
          <nav className="flex flex-col gap-2">
            <Link href="/contact" className="text-sm text-muted-foreground hover:text-foreground">
              Contact
            </Link>
            <Link href="/faq" className="text-sm text-muted-foreground hover:text-foreground">
              FAQ
            </Link>
          </nav>
        </div>

        <div className="flex flex-col gap-2">
          <h3 className="text-sm font-medium">Legal</h3>
          <nav className="flex flex-col gap-2">
            <Link href="/privacy" className="text-sm text-muted-foreground hover:text-foreground">
              Privacy Policy
            </Link>
            <Link href="/terms" className="text-sm text-muted-foreground hover:text-foreground">
              Terms of Service
            </Link>
          </nav>
        </div>
      </Container>

      <div className="border-t py-4">
        <Container>
          <p className="text-xs text-muted-foreground">
            &copy; {year} Grocery Shop. All rights reserved.
          </p>
        </Container>
      </div>
    </footer>
  )
}

export { SiteFooter }
