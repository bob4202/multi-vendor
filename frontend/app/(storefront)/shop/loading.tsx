import { Container } from "@/components/layout/container"
import { ProductCardGrid } from "@/components/product/product-card"
import { SHOP_GRID_CLASS } from "@/components/shop/shop-results"
import { Skeleton } from "@/components/ui/skeleton"

// First visit only - after that, filter changes are handled by the keyed
// Suspense around the results in page.tsx.
export default function ShopLoading() {
  return (
    <Container className="py-8">
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <Skeleton className="h-8 w-32" />
        <div className="flex gap-2">
          <Skeleton className="h-8 min-w-0 flex-1 sm:w-72 sm:flex-none" />
          <Skeleton className="h-8 w-24 sm:w-48" />
        </div>
      </div>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[200px_minmax(0,1fr)] lg:gap-8">
        <div className="flex gap-2 overflow-hidden lg:flex-col">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-8 w-24 lg:w-full" />
          ))}
        </div>
        <ProductCardGrid
          isLoading
          skeletonCount={6}
          className={SHOP_GRID_CLASS}
        />
      </div>
    </Container>
  )
}
