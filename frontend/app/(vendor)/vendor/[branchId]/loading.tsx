import { Skeleton } from "@/components/ui/skeleton"

// Fallback for the store and products sections (orders has its own).
export default function BranchSectionLoading() {
  return (
    <div className="flex flex-col gap-4">
      <Skeleton className="h-8 w-48" />
      <Skeleton className="h-4 w-72" />
      <Skeleton className="mt-2 h-40 w-full rounded-xl" />
      <Skeleton className="h-40 w-full rounded-xl" />
    </div>
  )
}
