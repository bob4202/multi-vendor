"use client"

import { useEffect, useState } from "react"

import { ProductImage } from "@/components/product/product-image"
import { AspectRatio } from "@/components/ui/aspect-ratio"
import {
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
  type CarouselApi,
} from "@/components/ui/carousel"
import { cn } from "@/lib/utils"

interface ProductGalleryProps {
  images: string[] | null
  alt: string | null
  className?: string
}

// Swipeable main image with a thumbnail strip kept in sync both ways
// (swipe -> thumbnail highlights, thumbnail tap -> carousel scrolls).
function ProductGallery({ images, alt, className }: ProductGalleryProps) {
  const [api, setApi] = useState<CarouselApi>()
  const [selected, setSelected] = useState(0)

  useEffect(() => {
    if (!api) return
    const onSelect = () => setSelected(api.selectedScrollSnap())
    onSelect()
    api.on("select", onSelect)
    return () => {
      api.off("select", onSelect)
    }
  }, [api])

  if (!images || images.length <= 1) {
    return (
      <div className={cn("overflow-hidden rounded-xl", className)}>
        <AspectRatio ratio={4 / 3}>
          <ProductImage src={images?.[0]} alt={alt} iconClassName="size-16" />
        </AspectRatio>
      </div>
    )
  }

  return (
    <div className={cn("flex flex-col gap-3", className)}>
      <Carousel setApi={setApi} className="overflow-hidden rounded-xl">
        <CarouselContent className="ml-0">
          {images.map((url, index) => (
            <CarouselItem key={`${url}-${index}`} className="pl-0">
              <AspectRatio ratio={4 / 3}>
                <ProductImage
                  src={url}
                  alt={`${alt ?? "Product"} - image ${index + 1}`}
                />
              </AspectRatio>
            </CarouselItem>
          ))}
        </CarouselContent>
        <CarouselPrevious className="left-3 bg-background/80 backdrop-blur-sm" />
        <CarouselNext className="right-3 bg-background/80 backdrop-blur-sm" />
      </Carousel>

      <div className="flex gap-2 overflow-x-auto pb-1">
        {images.map((url, index) => (
          <button
            key={`${url}-${index}`}
            type="button"
            aria-label={`Show image ${index + 1}`}
            aria-current={index === selected}
            onClick={() => api?.scrollTo(index)}
            className={cn(
              "size-16 shrink-0 overflow-hidden rounded-lg ring-2 ring-offset-2 ring-offset-background transition-all outline-none focus-visible:ring-ring",
              index === selected
                ? "ring-primary"
                : "opacity-60 ring-transparent hover:opacity-100"
            )}
          >
            <ProductImage src={url} alt={null} iconClassName="size-4" />
          </button>
        ))}
      </div>
    </div>
  )
}

export { ProductGallery }
