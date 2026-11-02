// components/ui/skeleton-button.tsx
import { cva, type VariantProps } from "class-variance-authority"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

const skeletonButtonVariants = cva("rounded-md", {
  variants: {
    size: {
      default: "h-8 w-20",
      xs: "h-6 w-16 rounded-[min(var(--radius-md),10px)]",
      sm: "h-7 w-18 rounded-[min(var(--radius-md),12px)]",
      lg: "h-9 w-24",
      icon: "size-8",
      "icon-xs": "size-6 rounded-[min(var(--radius-md),10px)]",
      "icon-sm": "size-7 rounded-[min(var(--radius-md),12px)]",
      "icon-lg": "size-9",
    },
  },
  defaultVariants: {
    size: "default",
  },
})

interface SkeletonButtonProps extends VariantProps<
  typeof skeletonButtonVariants
> {
  className?: string
}

function SkeletonButton({ size, className }: SkeletonButtonProps) {
  return (
    <Skeleton className={cn(skeletonButtonVariants({ size }), className)} />
  )
}

export { SkeletonButton, skeletonButtonVariants }
