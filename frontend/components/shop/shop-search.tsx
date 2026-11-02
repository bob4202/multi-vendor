"use client"

import { useEffect, useRef, useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Loader2, Search, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { shopHref } from "@/lib/shop-url"
import { cn } from "@/lib/utils"
import type { ProductSort } from "@/types/product"

const DEBOUNCE_MS = 300

interface ShopSearchProps {
  // Current URL values - the URL is the source of truth, this input just
  // writes to it.
  q?: string
  category?: string
  sort?: ProductSort
  className?: string
}

function ShopSearch({ q = "", category, sort, className }: ShopSearchProps) {
  const router = useRouter()
  const [value, setValue] = useState(q)
  const [isPending, startTransition] = useTransition()
  const lastPushed = useRef(q)

  // Back/forward or a "clear filters" link changed the URL under us -
  // follow it. Our own replace() landing is skipped, or it would clobber
  // whatever was typed while it was in flight.
  useEffect(() => {
    if (q === lastPushed.current) return
    lastPushed.current = q
    setValue(q)
  }, [q])

  useEffect(() => {
    const next = value.trim()
    if (next === lastPushed.current) return

    const timer = setTimeout(() => {
      lastPushed.current = next
      startTransition(() => {
        // replace, not push: one history entry per search, not per keystroke.
        router.replace(shopHref({ category, q: next || undefined, sort }), {
          scroll: false,
        })
      })
    }, DEBOUNCE_MS)
    return () => clearTimeout(timer)
  }, [value, category, sort, router])

  return (
    <div className={cn("relative", className)}>
      <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        type="search"
        value={value}
        onChange={(event) => setValue(event.target.value)}
        placeholder="Search products…"
        aria-label="Search products"
        className="pr-9 pl-8 [&::-webkit-search-cancel-button]:hidden"
      />
      <div className="absolute top-1/2 right-1 -translate-y-1/2">
        {isPending ? (
          <Loader2 className="mr-1.5 size-4 animate-spin text-muted-foreground" />
        ) : (
          value && (
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label="Clear search"
              onClick={() => setValue("")}
            >
              <X />
            </Button>
          )
        )}
      </div>
    </div>
  )
}

export { ShopSearch }
