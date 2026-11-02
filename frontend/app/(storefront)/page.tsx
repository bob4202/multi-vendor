import { Container } from "@/components/layout/container"
import {
  FEATURED_COUNT,
  FeaturedProducts,
} from "@/components/product/featured-products"
import { ProductCardGrid } from "@/components/product/product-card"
import { externalAPI } from "@/lib/external-api-service"

export default async function Page() {
  const [featured, { products }] = await Promise.all([
    externalAPI.getProducts({ featured: true, pageSize: FEATURED_COUNT }),
    externalAPI.getProducts({ pageSize: 100 }),
  ])

  return (
    <Container className="flex flex-col gap-10 py-8">
      <FeaturedProducts products={featured.products} />
      <section>
        <h1 className="mb-6 text-2xl font-semibold">Products</h1>
        {products.length === 0 ? (
          <p className="text-sm text-muted-foreground">No products found</p>
        ) : (
          <ProductCardGrid products={products} />
        )}
      </section>
    </Container>
  )
}
