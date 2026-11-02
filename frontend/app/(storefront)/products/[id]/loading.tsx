import { Container } from "@/components/layout/container"
import { AspectRatio } from "@/components/ui/aspect-ratio"
import { Skeleton } from "@/components/ui/skeleton"

export default function ProductLoading() {
  return (
    <Container className="flex flex-col gap-6 py-6 md:py-8">
      <Skeleton className="h-4 w-48" />
      <div className="grid grid-cols-1 gap-8 md:grid-cols-2 lg:gap-12">
        <AspectRatio ratio={4 / 3}>
          <Skeleton className="h-full w-full rounded-xl" />
        </AspectRatio>
        <div className="flex flex-col gap-4">
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-8 w-3/4" />
          <Skeleton className="h-8 w-24" />
          <Skeleton className="h-9 w-36" />
          <Skeleton className="mt-4 h-20 w-full" />
        </div>
      </div>
    </Container>
  )
}
