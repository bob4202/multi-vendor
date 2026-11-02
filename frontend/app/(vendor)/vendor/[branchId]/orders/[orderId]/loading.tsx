import { Skeleton } from "@/components/ui/skeleton"

export default function OrderLoading() {
  return (
    <div className="flex flex-col gap-4">
      <Skeleton className="h-7 w-20" />
      <Skeleton className="h-8 w-56" />
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <Skeleton className="h-72 rounded-xl" />
        <div className="flex flex-col gap-4">
          <Skeleton className="h-32 rounded-xl" />
          <Skeleton className="h-32 rounded-xl" />
        </div>
      </div>
    </div>
  )
}
