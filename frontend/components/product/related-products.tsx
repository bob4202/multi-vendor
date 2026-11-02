import { ProductCardGrid } from "@/components/product/product-card"
import { externalAPI } from "@/lib/external-api-service"

const RELATED_COUNT = 4
const GRID_CLASS = "grid-cols-2 lg:grid-cols-4"

interface RelatedProductsProps {
  productId: string
  categoryId: string | null
}

// Same-category picks, minus the product being viewed: featured ones first,
// then the rest of the category. Each list fetches enough extra that
// RELATED_COUNT are still left after dropping the viewed product and the
// featured picks already shown.
async function RelatedProducts({
  productId,
  categoryId,
}: RelatedProductsProps) {
  if (!categoryId) return null

  const [featured, rest] = await Promise.all([
    externalAPI.getProducts({
      category: categoryId,
      featured: true,
      pageSize: RELATED_COUNT + 1,
    }),
    externalAPI.getProducts({
      category: categoryId,
      pageSize: RELATED_COUNT * 2 + 1,
    }),
  ])
  const seen = new Set([productId])
  const related = [...featured.products, ...rest.products]
    .filter((p) => !seen.has(p.id) && seen.add(p.id))
    .slice(0, RELATED_COUNT)

  if (related.length === 0) return null

  return (
    <RelatedProductsSection>
      <ProductCardGrid
        products={related}
        className={`animate-in duration-300 fade-in-0 ${GRID_CLASS}`}
      />
    </RelatedProductsSection>
  )
}

function RelatedProductsSkeleton() {
  return (
    <RelatedProductsSection>
      <ProductCardGrid
        isLoading
        skeletonCount={RELATED_COUNT}
        className={GRID_CLASS}
      />
    </RelatedProductsSection>
  )
}

function RelatedProductsSection({ children }: { children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-4">
      <h2 className="text-lg font-semibold">You might also like</h2>
      {children}
    </section>
  )
}

export { RelatedProducts, RelatedProductsSkeleton }
