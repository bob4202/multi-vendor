import { Suspense, cache } from "react"
import type { Metadata } from "next"
import { notFound } from "next/navigation"

import { Container } from "@/components/layout/container"
import { ProductDetail } from "@/components/product/product-detail"
import {
  RelatedProducts,
  RelatedProductsSkeleton,
} from "@/components/product/related-products"
import { externalAPI } from "@/lib/external-api-service"

// Deduped per request - generateMetadata and the page both need it.
const getProduct = cache((id: string) => externalAPI.getProductById(id))

type Props = { params: Promise<{ id: string }> }

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params
  const product = await getProduct(id)
  if (!product) return {}
  return {
    title: product.name ?? "Product",
    description: product.description ?? undefined,
    openGraph: product.image_urls?.length
      ? { images: product.image_urls.slice(0, 1) }
      : undefined,
  }
}

export default async function ProductPage({ params }: Props) {
  const { id } = await params
  const product = await getProduct(id)

  if (!product) {
    notFound()
  }

  return (
    // Bottom padding on mobile clears the fixed purchase bar.
    <Container className="flex flex-col gap-16 py-6 pb-28 md:py-8 md:pb-12">
      <ProductDetail product={product} />
      <Suspense fallback={<RelatedProductsSkeleton />}>
        <RelatedProducts
          productId={product.id}
          categoryId={product.category_id}
        />
      </Suspense>
    </Container>
  )
}
