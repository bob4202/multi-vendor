"use client"

import { useTransition } from "react"
import { useRouter } from "next/navigation"
import { ArrowUpDown } from "lucide-react"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { shopHref } from "@/lib/shop-url"
import { cn } from "@/lib/utils"
import type { ProductSort } from "@/types/product"

// `short` is what the trigger shows on phones, where it shares a row with
// the search box.
const SORT_OPTIONS: { value: ProductSort; label: string; short: string }[] = [
  { value: "name_asc", label: "Name (A–Z)", short: "A–Z" },
  { value: "price_asc", label: "Price: low to high", short: "Price ↑" },
  { value: "price_desc", label: "Price: high to low", short: "Price ↓" },
]

interface ShopSortProps {
  // Current URL values - like ShopSearch, this only writes to the URL.
  sort: ProductSort
  category?: string
  q?: string
  className?: string
}

function ShopSort({ sort, category, q, className }: ShopSortProps) {
  const router = useRouter()
  const [isPending, startTransition] = useTransition()

  return (
    <Select
      items={SORT_OPTIONS}
      value={sort}
      onValueChange={(next) => {
        if (!next || next === sort) return
        startTransition(() => {
          // A new order starts back at page 1.
          router.push(shopHref({ category, q, sort: next }), { scroll: false })
        })
      }}
    >
      <SelectTrigger
        aria-label="Sort products"
        className={cn(
          "transition-opacity",
          isPending && "opacity-60",
          className
        )}
      >
        <ArrowUpDown className="text-muted-foreground" />
        <SelectValue>
          {(value: ProductSort) => {
            const option = SORT_OPTIONS.find((o) => o.value === value)
            return (
              <>
                <span className="sm:hidden">{option?.short}</span>
                <span className="hidden sm:inline">{option?.label}</span>
              </>
            )
          }}
        </SelectValue>
      </SelectTrigger>
      <SelectContent align="end" alignItemWithTrigger={false}>
        {SORT_OPTIONS.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

export { ShopSort }
