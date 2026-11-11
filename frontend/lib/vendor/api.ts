import "server-only"

import { cookies } from "next/headers"

import { createApiClient, orNull } from "@/lib/api-client"
import { config } from "@/lib/config"
import {
  VENDOR_LOGIN_PATH,
  VENDOR_SESSION_COOKIE,
} from "@/lib/vendor/constants"
import type {
  Closure,
  OrderDetail,
  OrderStatus,
  OrderSummary,
  Paginated,
  StoreInfo,
  SyncState,
  VendorSession,
} from "@/types/vendor"
import type { Category } from "@/types/category"
import type { Product } from "@/types/product"

// Vendor portal API client (/v1/vendor). Server-only: the session token
// lives in an httpOnly cookie and is attached by createApiClient.

const { request: vendorFetch } = createApiClient({
  prefix: "/v1/vendor",
  cookie: VENDOR_SESSION_COOKIE,
  loginPath: VENDOR_LOGIN_PATH,
})

const branchPath = (branchId: string, path = "") =>
  `/branches/${encodeURIComponent(branchId)}${path}`

export interface LoginResponse {
  token: string
  expires_at: string
}

export interface InviteInfo {
  email: string
  name: string
  vendor_name: string
  expires_at: string
}

export type OrderListFilter = "active" | "done" | "all"

export const vendorAPI = {
  // The branch's live order stream (SSE), for the route handler that relays
  // it to the browser with the session token added.
  openBranchStream: async (branchId: string, signal: AbortSignal) => {
    const token = (await cookies()).get(VENDOR_SESSION_COOKIE)?.value
    const baseUrl = (config.api.baseUrl ?? "").replace(/\/+$/, "")
    return fetch(`${baseUrl}/v1/vendor${branchPath(branchId, "/events")}`, {
      headers: {
        Authorization: `Bearer ${token ?? ""}`,
        Accept: "text/event-stream",
      },
      cache: "no-store",
      signal,
    })
  },

  login: (email: string, password: string, userAgent: string) =>
    vendorFetch<LoginResponse>("/auth/login", {
      method: "POST",
      body: { email, password },
      token: null,
      headers: { "User-Agent": userAgent },
    }),

  logout: (token: string) =>
    vendorFetch<void>("/auth/logout", { method: "POST", token }),

  me: (token: string) => vendorFetch<VendorSession>("/me", { token }),

  getInvite: (token: string) =>
    vendorFetch<InviteInfo>(`/invites/${encodeURIComponent(token)}`, {
      token: null,
    }),

  acceptInvite: (token: string, password: string, userAgent: string) =>
    vendorFetch<LoginResponse>(`/invites/${encodeURIComponent(token)}/accept`, {
      method: "POST",
      body: { password },
      token: null,
      headers: { "User-Agent": userAgent },
    }),

  listOrders: (
    branchId: string,
    { status, page }: { status: OrderListFilter; page: number }
  ) =>
    vendorFetch<Paginated<OrderSummary>>(
      branchPath(branchId, `/orders?status=${status}&page=${page}&page_size=25`)
    ),

  getOrder: (branchId: string, orderId: string) =>
    orNull(
      vendorFetch<OrderDetail>(
        branchPath(branchId, `/orders/${encodeURIComponent(orderId)}`)
      )
    ),

  changeOrderStatus: (
    branchId: string,
    orderId: string,
    body: {
      status: OrderStatus
      expected_status: OrderStatus
      reason?: string
      // Handing over a collection order: the customer's code, or why not.
      collection_code?: string
      code_override_reason?: string
    }
  ) =>
    vendorFetch<OrderDetail>(
      branchPath(branchId, `/orders/${encodeURIComponent(orderId)}/status`),
      { method: "POST", body }
    ),

  // Open collection orders with this code (normally exactly one).
  lookupOrderByCode: (branchId: string, code: string) =>
    orNull(
      vendorFetch<{ data: OrderSummary[] }>(
        branchPath(branchId, `/orders/lookup?code=${encodeURIComponent(code)}`)
      )
    ),

  // substitute null = just remove the line.
  markItemUnavailable: (
    branchId: string,
    orderId: string,
    itemId: string,
    substitute: { product_id: string; quantity: number } | null
  ) =>
    vendorFetch<OrderDetail>(
      branchPath(
        branchId,
        `/orders/${encodeURIComponent(orderId)}/items/${encodeURIComponent(itemId)}/unavailable`
      ),
      { method: "POST", body: { substitute } }
    ),

  withdrawSubstitution: (branchId: string, orderId: string, itemId: string) =>
    vendorFetch<OrderDetail>(
      branchPath(
        branchId,
        `/orders/${encodeURIComponent(orderId)}/items/${encodeURIComponent(itemId)}/substitution`
      ),
      { method: "DELETE" }
    ),

  // Owners and managers. 202: the worker gives the money back shortly.
  refundOrder: (
    branchId: string,
    orderId: string,
    body: { amount: number; reason: string }
  ) =>
    vendorFetch<OrderDetail>(
      branchPath(branchId, `/orders/${encodeURIComponent(orderId)}/refunds`),
      { method: "POST", body }
    ),

  getStoreInfo: (branchId: string) =>
    vendorFetch<StoreInfo & { name: string }>(branchPath(branchId, "/info")),

  saveStoreInfo: (branchId: string, info: StoreInfo) =>
    vendorFetch<StoreInfo & { name: string }>(branchPath(branchId, "/info"), {
      method: "PUT",
      body: info,
    }),

  listClosures: (branchId: string) =>
    vendorFetch<Closure[]>(branchPath(branchId, "/closures")),

  addClosure: (branchId: string, closure: Omit<Closure, "id">) =>
    vendorFetch<Closure>(branchPath(branchId, "/closures"), {
      method: "POST",
      body: closure,
    }),

  deleteClosure: (branchId: string, closureId: number) =>
    vendorFetch<void>(branchPath(branchId, `/closures/${closureId}`), {
      method: "DELETE",
    }),

  listProducts: (
    branchId: string,
    {
      search,
      category,
      page,
    }: { search?: string; category?: string; page: number }
  ) => {
    const params = new URLSearchParams({ page: String(page), page_size: "25" })
    if (search) params.set("q", search)
    if (category) params.set("category", category)
    return vendorFetch<Paginated<Product>>(
      branchPath(branchId, `/products?${params.toString()}`)
    )
  },

  listCategories: (branchId: string) =>
    vendorFetch<Category[]>(branchPath(branchId, "/categories")),

  syncState: (branchId: string) =>
    vendorFetch<SyncState[]>(branchPath(branchId, "/sync-state")),

  triggerSync: (branchId: string) =>
    vendorFetch<{ status: string }>(branchPath(branchId, "/sync"), {
      method: "POST",
    }),
}
