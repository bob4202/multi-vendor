import Link from "next/link"

import { AddToCartButton } from "@/components/cart/add-to-cart-button"
import { Price } from "@/components/common/price"
import { ProductGallery } from "@/components/product/product-gallery"
import { ProductPurchaseBar } from "@/components/product/product-purchase-bar"
import { Badge } from "@/components/ui/badge"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import { Separator } from "@/components/ui/separator"
import { shopHref } from "@/lib/shop-url"
import { cn } from "@/lib/utils"
import type { Product } from "@/types/product"

interface ProductDetailProps {
  product: Product
}

function ProductDetail({ product }: ProductDetailProps) {
  const {
    name,
    description,
    category_id,
    category_name,
    price,
    image_urls,
    is_upsell,
    can_be_sold,
    deleted,
  } = product

  const unavailable = can_be_sold === false
  const purchasable = !deleted && !unavailable

  return (
    <div className="flex flex-col gap-6">
      <Breadcrumb>
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink render={<Link href="/shop" />}>Shop</BreadcrumbLink>
          </BreadcrumbItem>
          {category_id && category_name && (
            <>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbLink
                  render={<Link href={shopHref({ category: category_id })} />}
                >
                  {category_name}
                </BreadcrumbLink>
              </BreadcrumbItem>
            </>
          )}
          <BreadcrumbSeparator />
          <BreadcrumbItem className="min-w-0">
            <BreadcrumbPage className="line-clamp-1">
              {name ?? "Product"}
            </BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>

      <div className="grid grid-cols-1 gap-8 md:grid-cols-2 lg:gap-12">
        <ProductGallery images={image_urls} alt={name} className="min-w-0" />

        <div className="flex min-w-0 animate-in flex-col gap-5 duration-300 fade-in-0 slide-in-from-bottom-2">
          {(deleted || unavailable || is_upsell) && (
            <div className="flex flex-wrap gap-1">
              {deleted && <Badge variant="destructive">Removed</Badge>}
              {!deleted && unavailable && (
                <Badge variant="secondary">Unavailable</Badge>
              )}
              {is_upsell && <Badge variant="outline">Featured</Badge>}
            </div>
          )}

          <div className="flex flex-col gap-1">
            {category_name && (
              <p className="text-sm text-muted-foreground">{category_name}</p>
            )}
            <h1
              className={cn(
                "text-2xl font-semibold tracking-tight sm:text-3xl",
                !name && "font-normal text-muted-foreground italic"
              )}
            >
              {name ?? "Unnamed product"}
            </h1>
          </div>

          <Price pence={price} className="text-2xl font-semibold" />

          {/* Hidden on mobile - the sticky purchase bar takes over there. */}
          <div className="hidden md:block">
            {purchasable ? (
              <AddToCartButton product={product} size="lg" openCartOnAdd />
            ) : (
              <p className="text-sm text-muted-foreground">
                {deleted
                  ? "This product is no longer available."
                  : "Currently unavailable."}
              </p>
            )}
          </div>

          <Separator />

          <div className="flex flex-col gap-2">
            <h2 className="text-sm font-semibold">Description</h2>
            <p className="text-sm leading-relaxed whitespace-pre-line text-muted-foreground">
              {description ?? "No description provided."}
            </p>
          </div>
        </div>
      </div>

      {purchasable && <ProductPurchaseBar product={product} />}
    </div>
  )
}

export { ProductDetail }
