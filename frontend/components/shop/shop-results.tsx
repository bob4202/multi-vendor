import Link from "next/link"
import { SearchX } from "lucide-react"

import { EmptyState } from "@/components/common/empty-state"
import { ProductCardGrid } from "@/components/product/product-card"
import { Button } from "@/components/ui/button"
import { Pagination } from "@/components/ui/pagination"
import { externalAPI } from "@/lib/external-api-service"
import { shopHref } from "@/lib/shop-url"
import type { ProductSort } from "@/types/product"

export const SHOP_PAGE_SIZE = 12
// Overrides every breakpoint of ProductCardGrid's default (which assumes a
// full-width page) - the shop has a 200px sidebar from lg up, so it stays
// at 3 columns there and only goes to 4 once xl gives the cards room.
export const SHOP_GRID_CLASS =
  "grid-cols-2 md:grid-cols-3 lg:grid-cols-3 xl:grid-cols-4"

interface ShopResultsProps {
  category?: string
  q?: string
  sort: ProductSort
  page: number
}

// Async on purpose: the shop page wraps this in a Suspense keyed on the
// filters, so changing category/search/sort/page swaps just this area for a
// skeleton while the header, search box and category nav stay put.
async function ShopResults({ category, q, sort, page }: ShopResultsProps) {
  const { products, totalPages, totalCount } = await externalAPI.getProducts({
    category,
    search: q,
    sort,
    page,
    pageSize: SHOP_PAGE_SIZE,
  })

  if (products.length === 0) {
    const filtered = Boolean(q || category)
    return (
      <EmptyState
        icon={SearchX}
        title={q ? `No results for “${q}”` : "No products here yet"}
        description={
          filtered
            ? "Try a different search or browse all products."
            : "Check back soon - new stock is added regularly."
        }
        action={
          filtered && (
            <Button
              variant="outline"
              nativeButton={false}
              render={<Link href={shopHref({})} />}
            >
              Clear filters
            </Button>
          )
        }
      />
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-muted-foreground">
        {totalCount} {totalCount === 1 ? "product" : "products"}
        {q && <> matching “{q}”</>}
      </p>
      <ProductCardGrid
        products={products}
        className={`animate-in duration-300 fade-in-0 ${SHOP_GRID_CLASS}`}
      />
      <Pagination
        className="mt-4"
        page={page}
        totalPages={totalPages}
        hrefForPage={(p) => shopHref({ category, q, sort, page: p })}
      />
    </div>
  )
}

export { ShopResults }
