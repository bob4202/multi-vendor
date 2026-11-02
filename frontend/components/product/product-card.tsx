"use client"

import Link from "next/link"

import { AddToCartButton } from "@/components/cart/add-to-cart-button"
import { Price } from "@/components/common/price"
import { ProductImage } from "@/components/product/product-image"
import { AspectRatio } from "@/components/ui/aspect-ratio"
import { Badge } from "@/components/ui/badge"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import {
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
} from "@/components/ui/carousel"
import type { Product } from "@/types/product"

export type { Product }

// ---------------------------------------------------------------------------
// ProductCard
// ---------------------------------------------------------------------------

interface ProductCardProps {
  product: Product
  className?: string
}

function ProductCard({ product, className }: ProductCardProps) {
  const {
    id,
    name,
    description,
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
    <Card
      className={cn(
        "relative min-w-0 gap-0 overflow-hidden py-0 transition-colors hover:bg-accent/40",
        deleted && "opacity-50",
        className
      )}
    >
      {/* Full-card "stretched link" so clicking anywhere opens the product -
          sits above plain content (image, text) but below the controls
          below, which get their own stacking context to stay clickable. */}
      <Link
        href={`/products/${id}`}
        className="absolute inset-0 z-10 rounded-xl"
        aria-label={`View ${name ?? "product"} details`}
      />

      <AspectRatio ratio={4 / 3} className="relative bg-muted">
        {image_urls && image_urls.length > 1 ? (
          <Carousel className="h-full w-full">
            <CarouselContent className="ml-0 h-full">
              {image_urls.map((url, index) => (
                <CarouselItem key={`${url}-${index}`} className="h-full pl-0">
                  <ProductImage src={url} alt={name} />
                </CarouselItem>
              ))}
            </CarouselContent>
            <CarouselPrevious className="left-2 z-20 size-6 bg-background/80 backdrop-blur-sm" />
            <CarouselNext className="right-2 z-20 size-6 bg-background/80 backdrop-blur-sm" />
          </Carousel>
        ) : (
          <ProductImage
            src={image_urls?.[0]}
            alt={name}
            className="transition-transform duration-300 group-hover/card:scale-[1.03]"
          />
        )}
        {(deleted || unavailable || is_upsell) && (
          <div className="absolute top-2 left-2 flex flex-wrap gap-1">
            {deleted && <Badge variant="destructive">Deleted</Badge>}
            {!deleted && unavailable && (
              <Badge variant="secondary">Unavailable</Badge>
            )}
            {is_upsell && (
              <Badge variant="outline" className="bg-background">
                Featured
              </Badge>
            )}
          </div>
        )}
      </AspectRatio>

      <CardHeader className="gap-1 px-3 pt-3">
        <CardTitle
          className={cn(
            "line-clamp-1 text-sm",
            !name && "font-normal text-muted-foreground italic"
          )}
        >
          {name ?? "Unnamed product"}
        </CardTitle>
        {category_name && (
          <CardDescription className="line-clamp-1">
            {category_name}
          </CardDescription>
        )}
      </CardHeader>

      <CardContent className="px-3 py-2">
        {description ? (
          <p className="line-clamp-3 text-xs text-muted-foreground">
            {description}
          </p>
        ) : (
          <span className="text-xs text-muted-foreground">
            No Description Provided
          </span>
        )}
      </CardContent>

      <CardFooter className="mt-auto flex flex-wrap items-center justify-between gap-2 px-3 pb-3">
        <Price pence={price} className="text-sm font-semibold" />
        {purchasable ? (
          <AddToCartButton
            product={product}
            size="sm"
            className="relative z-20"
          />
        ) : (
          <span className="text-xs text-muted-foreground">
            {deleted ? "Removed" : "Unavailable"}
          </span>
        )}
      </CardFooter>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// ProductCardSkeleton — mirrors ProductCard's exact box model so nothing
// shifts when real data swaps in.
// ---------------------------------------------------------------------------

function ProductCardSkeleton({ className }: { className?: string }) {
  return (
    <Card className={cn("gap-0 overflow-hidden py-0", className)}>
      <AspectRatio ratio={4 / 3}>
        <Skeleton className="h-full w-full rounded-none" />
      </AspectRatio>

      <CardHeader className="gap-2 px-3 pt-3">
        <Skeleton className="h-4 w-3/4" />
        <Skeleton className="h-3 w-1/3" />
      </CardHeader>

      <CardContent className="px-3 py-2">
        <Skeleton className="h-3 w-2/5" />
      </CardContent>

      <CardFooter className="mt-auto px-3 pb-3">
        <Skeleton className="h-4 w-16" />
      </CardFooter>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// ProductCardGrid — convenience wrapper for the common loading/loaded switch.
// Optional; delete if you'd rather compose the grid yourself.
// ---------------------------------------------------------------------------

interface ProductCardGridProps {
  products?: Product[]
  isLoading?: boolean
  skeletonCount?: number
  className?: string
}

function ProductCardGrid({
  products,
  isLoading,
  skeletonCount = 8,
  className,
}: ProductCardGridProps) {
  return (
    <div
      className={cn(
        "grid grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4 lg:grid-cols-4",
        className
      )}
    >
      {isLoading
        ? Array.from({ length: skeletonCount }).map((_, i) => (
            <ProductCardSkeleton key={i} />
          ))
        : products?.map((product) => (
            <ProductCard key={product.id} product={product} />
          ))}
    </div>
  )
}

export { ProductCard, ProductCardSkeleton, ProductCardGrid }
