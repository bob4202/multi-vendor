import type { ProductSort } from "@/types/product"

export const DEFAULT_SORT: ProductSort = "name_asc"

const SORTS: readonly ProductSort[] = ["name_asc", "price_asc", "price_desc"]

// Unknown or missing ?sort= values fall back to the default instead of
// erroring - the API would 400 on them.
export function parseSort(value: string | undefined): ProductSort {
  return SORTS.find((s) => s === value) ?? DEFAULT_SORT
}

export interface ShopParams {
  category?: string
  q?: string
  sort?: ProductSort
  page?: number
}

// Single source of truth for /shop URLs, so the category nav, search box,
// sort and pagination all preserve each other's params the same way.
// Changing a filter should reset to page 1 - callers just omit `page`.
export function shopHref({ category, q, sort, page }: ShopParams) {
  const params = new URLSearchParams()
  if (category) params.set("category", category)
  if (q) params.set("q", q)
  if (sort && sort !== DEFAULT_SORT) params.set("sort", sort)
  if (page && page > 1) params.set("page", String(page))
  const qs = params.toString()
  return qs ? `/shop?${qs}` : "/shop"
}
