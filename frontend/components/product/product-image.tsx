import { Package } from "lucide-react"

import { cn } from "@/lib/utils"

interface ProductImageProps {
  src: string | null | undefined
  alt: string | null
  className?: string
  iconClassName?: string
}

// One product photo filling its parent, or a placeholder icon when there's
// none. Size it from the outside (AspectRatio, size-20, ...).
function ProductImage({
  src,
  alt,
  className,
  iconClassName,
}: ProductImageProps) {
  if (!src) {
    return (
      <div
        className={cn(
          "flex h-full w-full items-center justify-center bg-muted",
          className
        )}
      >
        <Package
          className={cn("size-8 text-muted-foreground/40", iconClassName)}
        />
      </div>
    )
  }

  return (
    <img
      src={src}
      alt={alt ?? "Product"}
      className={cn("h-full w-full bg-muted object-cover", className)}
    />
  )
}

export { ProductImage }
