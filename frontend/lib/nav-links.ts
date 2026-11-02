export interface NavLink {
  href: string
  label: string
}

// Placeholder routes - swap/extend once the real shop/category pages exist.
export const NAV_LINKS: NavLink[] = [
  { href: "/", label: "Home" },
  { href: "/shop", label: "Shop" },
  { href: "/categories", label: "Categories" },
  { href: "/about", label: "About" },
]
