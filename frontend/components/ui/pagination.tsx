import Link from "next/link"
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  MoreHorizontalIcon,
} from "lucide-react"

import { buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"

// Href-based (not onClick/state) so it works from a plain server component
// (next/link, so paging is a client-side navigation, not a full reload) -
// the caller (whatever list this paginates) owns the URL scheme and just
// hands back a link for each page number via `hrefForPage`.
interface PaginationProps {
  page: number
  totalPages: number
  hrefForPage: (page: number) => string
  className?: string
}

function getPageNumbers(
  current: number,
  total: number
): (number | "ellipsis")[] {
  const siblingCount = 1
  const left = Math.max(2, current - siblingCount)
  const right = Math.min(total - 1, current + siblingCount)

  const pages: (number | "ellipsis")[] = [1]
  if (left > 2) pages.push("ellipsis")
  for (let i = left; i <= right; i++) pages.push(i)
  if (right < total - 1) pages.push("ellipsis")
  if (total > 1) pages.push(total)

  return pages
}

function Pagination({
  page,
  totalPages,
  hrefForPage,
  className,
}: PaginationProps) {
  if (totalPages <= 1) return null

  return (
    <nav
      aria-label="Pagination"
      data-slot="pagination"
      className={cn("flex items-center justify-center gap-1", className)}
    >
      <PaginationArrow
        direction="prev"
        disabled={page <= 1}
        href={hrefForPage(page - 1)}
      />

      {getPageNumbers(page, totalPages).map((n, i) =>
        n === "ellipsis" ? (
          <span
            key={`ellipsis-${i}`}
            aria-hidden
            className="flex size-8 items-center justify-center text-muted-foreground"
          >
            <MoreHorizontalIcon className="size-4" />
          </span>
        ) : (
          <Link
            key={n}
            href={hrefForPage(n)}
            aria-current={n === page ? "page" : undefined}
            className={cn(
              buttonVariants({
                variant: n === page ? "default" : "outline",
                size: "icon-sm",
              })
            )}
          >
            {n}
          </Link>
        )
      )}

      <PaginationArrow
        direction="next"
        disabled={page >= totalPages}
        href={hrefForPage(page + 1)}
      />
    </nav>
  )
}

function PaginationArrow({
  direction,
  disabled,
  href,
}: {
  direction: "prev" | "next"
  disabled: boolean
  href: string
}) {
  const Icon = direction === "prev" ? ChevronLeftIcon : ChevronRightIcon
  const label = direction === "prev" ? "Previous page" : "Next page"
  const classes = cn(buttonVariants({ variant: "outline", size: "icon-sm" }))

  if (disabled) {
    return (
      <span
        aria-hidden
        className={cn(classes, "pointer-events-none opacity-50")}
      >
        <Icon />
      </span>
    )
  }

  return (
    <Link href={href} aria-label={label} className={classes}>
      <Icon />
    </Link>
  )
}

export { Pagination }
