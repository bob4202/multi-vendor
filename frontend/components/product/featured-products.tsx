import Link from "next/link"
import { ArrowRight } from "lucide-react"

import { ProductCardGrid } from "@/components/product/product-card"
import type { Product } from "@/types/product"

export const FEATURED_COUNT = 8

interface FeaturedProductsProps {
  products: Product[]
}

// Products the branch marked as featured. Hidden entirely when there are
// none, rather than showing an empty section.
function FeaturedProducts({ products }: FeaturedProductsProps) {
  if (products.length === 0) return null

  return (
    <section className="flex flex-col gap-4">
      <div className="flex items-baseline justify-between gap-4">
        <h2 className="text-xl font-semibold">Featured</h2>
        <Link
          href="/shop"
          className="inline-flex items-center gap-1 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          Shop all
          <ArrowRight className="size-4" />
        </Link>
      </div>
      <ProductCardGrid
        products={products}
        className="grid-cols-2 md:grid-cols-4"
      />
    </section>
  )
}

export { FeaturedProducts }
