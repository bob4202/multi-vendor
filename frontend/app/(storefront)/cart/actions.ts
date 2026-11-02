"use server"

import { externalAPI } from "@/lib/external-api-service"
import type { Product } from "@/types/product"

// A pool, not just what's on screen: the row shows a few at a time and
// refills from here as products are added to the cart.
const SUGGESTION_COUNT = 16
// Categories of the cart to draw from; more would mean more API calls for
// a row that only shows a handful of products.
const MAX_CATEGORIES = 2

// "Add to your order" picks for the cart and checkout: the branch's
// featured products first, then more from the categories already in the
// cart. Anything in the cart or not for sale is left out.
export async function getCartSuggestions(
  cartIds: string[],
  categoryIds: string[]
): Promise<Product[]> {
  const categories = [...new Set(categoryIds)].slice(0, MAX_CATEGORIES)
  const results = await Promise.all([
    externalAPI.getProducts({ featured: true, pageSize: SUGGESTION_COUNT }),
    ...categories.map((category) =>
      externalAPI.getProducts({ category, pageSize: SUGGESTION_COUNT })
    ),
  ])

  const seen = new Set(cartIds)
  return results
    .flatMap((r) => r.products)
    .filter(
      (p) =>
        !p.deleted &&
        p.can_be_sold !== false &&
        p.price !== null &&
        !seen.has(p.id) &&
        !!seen.add(p.id)
    )
    .slice(0, SUGGESTION_COUNT)
}
