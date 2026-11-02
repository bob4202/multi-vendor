import Link from "next/link"
import { ChevronLeft, ChevronRight } from "lucide-react"

import { Button } from "@/components/ui/button"

// Previous / "Page x of y" / Next. Renders nothing for a single page.
function SimplePagination({
  page,
  totalPages,
  hrefForPage,
}: {
  page: number
  totalPages: number
  hrefForPage: (page: number) => string
}) {
  if (totalPages <= 1) return null

  return (
    <div className="mt-6 flex items-center justify-between gap-2">
      {page > 1 ? (
        <Button
          variant="outline"
          nativeButton={false}
          render={<Link href={hrefForPage(page - 1)} />}
        >
          <ChevronLeft />
          Previous
        </Button>
      ) : (
        <span />
      )}
      <span className="text-sm text-muted-foreground">
        Page {page} of {totalPages}
      </span>
      {page < totalPages ? (
        <Button
          variant="outline"
          nativeButton={false}
          render={<Link href={hrefForPage(page + 1)} />}
        >
          Next
          <ChevronRight />
        </Button>
      ) : (
        <span />
      )}
    </div>
  )
}

export { SimplePagination }
