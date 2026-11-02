import { Suspense } from "react"
import type { Metadata } from "next"

import { Container } from "@/components/layout/container"
import { ProductCardGrid } from "@/components/product/product-card"
import { CategoryNav } from "@/components/shop/category-nav"
import {
  SHOP_GRID_CLASS,
  SHOP_PAGE_SIZE,
  ShopResults,
} from "@/components/shop/shop-results"
import { ShopSearch } from "@/components/shop/shop-search"
import { ShopSort } from "@/components/shop/shop-sort"
import { Skeleton } from "@/components/ui/skeleton"
import { externalAPI } from "@/lib/external-api-service"
import { parseSort } from "@/lib/shop-url"

export const metadata: Metadata = { title: "Shop" }

export default async function ShopPage({
  searchParams,
}: {
  searchParams: Promise<{
    category?: string
    q?: string
    sort?: string
    page?: string
  }>
}) {
  const { category, q, sort: sortParam, page: pageParam } = await searchParams
  const sort = parseSort(sortParam)
  const page = Math.max(1, Number(pageParam) || 1)
  const categories = await externalAPI.getCategories()
  const activeCategory = categories.find((c) => c.id === category)

  return (
    <Container className="py-8">
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <h1 className="text-2xl font-semibold">
          {activeCategory?.name ?? "Shop"}
        </h1>
        <div className="flex gap-2">
          <ShopSearch
            q={q}
            category={category}
            sort={sort}
            className="min-w-0 flex-1 sm:w-72 sm:flex-none"
          />
          <ShopSort
            sort={sort}
            category={category}
            q={q}
            className="shrink-0 sm:w-48"
          />
        </div>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[200px_minmax(0,1fr)] lg:gap-8">
        <aside className="min-w-0 lg:sticky lg:top-20 lg:self-start">
          <CategoryNav
            categories={categories}
            active={category}
            q={q}
            sort={sort}
          />
        </aside>

        <Suspense
          key={`${category ?? ""}|${q ?? ""}|${sort}|${page}`}
          fallback={<ShopResultsSkeleton />}
        >
          <ShopResults category={category} q={q} sort={sort} page={page} />
        </Suspense>
      </div>
    </Container>
  )
}

function ShopResultsSkeleton() {
  return (
    <div className="flex flex-col gap-4">
      <Skeleton className="h-5 w-24" />
      <ProductCardGrid
        isLoading
        skeletonCount={SHOP_PAGE_SIZE / 2}
        className={SHOP_GRID_CLASS}
      />
    </div>
  )
}
