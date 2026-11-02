import type { Product } from "@/components/product/product-card"

// Products are shared across branches but price/availability are per
// branch, so each branch gets its own cart - otherwise a cart filled on one
// branch's storefront would carry that branch's prices (or items it doesn't
// sell) into another's on the same origin.
export function cartStorageKey(branchId: string) {
  return `cart:${branchId}`
}

// The full product is snapshotted into the cart item (not just an id) so the
// cart page can render without a product lookup - once there's a backend,
// this is also what makes add-to-cart idempotent to sync from.
export interface CartItem {
  product: Product
  quantity: number
}

// The order this tab is paying for. Its cart is kept until the order page
// sees the order paid, so backing out of the payment page loses nothing.
const PAYING_ORDER_KEY = "paying-order"

export function rememberPayingOrder(orderId: string) {
  try {
    sessionStorage.setItem(PAYING_ORDER_KEY, orderId)
  } catch {
    // storage blocked: the cart just stays until the customer empties it
  }
}

// True once, when orderId is the order this tab's cart was paying for.
export function takePayingOrder(orderId: string) {
  try {
    if (sessionStorage.getItem(PAYING_ORDER_KEY) !== orderId) return false
    sessionStorage.removeItem(PAYING_ORDER_KEY)
    return true
  } catch {
    return false
  }
}
