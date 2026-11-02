import Link from "next/link"

import { shopHref } from "@/lib/shop-url"
import { cn } from "@/lib/utils"
import type { Category } from "@/types/category"
import type { ProductSort } from "@/types/product"

interface CategoryNavProps {
  categories: Category[]
  active?: string
  // Kept when switching category, so a search narrows across categories.
  q?: string
  sort?: ProductSort
}

// Horizontal scrolling chips on phones and tablets, a vertical sidebar list
// from lg up (on a portrait iPad the sidebar would cost a product column) -
// same links, one component.
function CategoryNav({ categories, active, q, sort }: CategoryNavProps) {
  const options = [{ id: undefined, name: "All products" }, ...categories]

  return (
    <nav aria-label="Categories" className="-mx-4 sm:-mx-6 lg:mx-0">
      <span className="hidden px-3 pb-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase lg:block">
        Categories
      </span>
      <ul className="flex [scrollbar-width:none] gap-2 overflow-x-auto px-4 pb-1 sm:px-6 lg:flex-col lg:gap-1 lg:overflow-visible lg:px-0">
        {options.map((cat) => {
          const isActive = cat.id === active
          return (
            <li key={cat.id ?? "all"} className="shrink-0">
              <Link
                href={shopHref({ category: cat.id, q, sort })}
                aria-current={isActive ? "page" : undefined}
                className={cn(
                  "block rounded-full border px-3 py-1.5 text-sm font-medium whitespace-nowrap transition-colors lg:rounded-md lg:border-transparent lg:py-2",
                  isActive
                    ? "border-primary bg-primary text-primary-foreground lg:border-transparent lg:bg-muted lg:text-foreground"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground"
                )}
              >
                {cat.name}
              </Link>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}

export { CategoryNav }
