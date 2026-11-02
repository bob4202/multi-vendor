import "server-only"

import { cookies } from "next/headers"

import { config } from "./config"
import { CUSTOMER_SESSION_COOKIE } from "@/lib/customer/constants"
import type { Product, ProductSort } from "@/types/product"
import type { Category } from "@/types/category"
import type { BranchInfo } from "@/types/branch"
import type { ApiIssue } from "@/types/api"
import type { Customer } from "@/types/customer"
import type {
  CustomerOrderSummary,
  Order,
  OrderIssue,
  PlaceOrderRequest,
} from "@/types/order"

interface PaginatedResponse<T> {
  data: T[]
  page: number
  page_size: number
  total_count: number
  total_pages: number
}

// Returns the parsed JSON body directly - callers don't (and shouldn't) see
// the raw Response. A 404 resolves to null instead of throwing, since "not
// found" is an expected outcome (e.g. a bad product id), not a failure.
async function fetchExternal<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T | null> {
  // config.api.baseUrl carries a trailing slash (see .env) and endpoint
  // starts with one - strip it so we don't end up requesting "//v1/...".
  const baseUrl = (config.api.baseUrl ?? "").replace(/\/+$/, "")

  const res = await fetch(`${baseUrl}${endpoint}`, {
    ...options,
    headers: {
      Authorization: `Bearer ${config.api.storefrontKey}`,
      "Content-Type": "application/json",
      ...options.headers,
    },
  })

  if (res.status === 404) {
    return null
  }
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText}`)
  }
  return res.json()
}

// The signed-in customer's session, sent next to the storefront key. Only
// customer-specific calls add it, so catalogue pages stay cacheable.
async function customerHeaders(): Promise<Record<string, string>> {
  const token = (await cookies()).get(CUSTOMER_SESSION_COOKIE)?.value
  return token ? { "X-Customer-Session": token } : {}
}

// Like fetchExternal, but hands back non-2xx responses instead of throwing -
// placing an order needs the 422/409 bodies to tell the customer what's wrong.
async function requestExternal<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<{ status: number; body: T }> {
  const baseUrl = (config.api.baseUrl ?? "").replace(/\/+$/, "")
  const res = await fetch(`${baseUrl}${endpoint}`, {
    cache: "no-store",
    ...options,
    headers: {
      Authorization: `Bearer ${config.api.storefrontKey}`,
      "Content-Type": "application/json",
      ...options.headers,
    },
  })
  return { status: res.status, body: (await res.json().catch(() => ({}))) as T }
}

export type ApiResult<T> =
  | { ok: true; data: T }
  | { ok: false; status: number; error: string; issues: ApiIssue[] }

// requestExternal shaped as ok/error, for calls whose errors the UI shows.
async function apiResult<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<ApiResult<T>> {
  const { status, body } = await requestExternal<
    T & { error?: string; issues?: ApiIssue[] }
  >(endpoint, options)
  if (status >= 200 && status < 300) return { ok: true, data: body }
  return {
    ok: false,
    status,
    error: body.error ?? "Something went wrong.",
    issues: body.issues ?? [],
  }
}

export interface PushSubscriptionJSON {
  endpoint: string
  keys: { p256dh: string; auth: string }
}

export type PlaceOrderResult =
  | { ok: true; order: Order }
  | { ok: false; status: number; error: string; issues: OrderIssue[] }

export interface ProductsQuery {
  category?: string
  search?: string
  // Only products the branch marked as featured (is_upsell).
  featured?: boolean
  sort?: ProductSort
  page?: number
  pageSize?: number
}

export interface ProductsResult {
  products: Product[]
  page: number
  pageSize: number
  totalCount: number
  totalPages: number
}

export const externalAPI = {
  getProducts: async (query: ProductsQuery = {}): Promise<ProductsResult> => {
    const params = new URLSearchParams()
    if (query.category) params.set("category", query.category)
    if (query.search) params.set("q", query.search)
    if (query.featured) params.set("featured", "true")
    if (query.sort) params.set("sort", query.sort)
    params.set("page", String(query.page ?? 1))
    params.set("page_size", String(query.pageSize ?? 20))

    const res = await fetchExternal<PaginatedResponse<Product>>(
      `/v1/store/products?${params.toString()}`
    )

    return {
      products: res?.data ?? [],
      page: res?.page ?? query.page ?? 1,
      pageSize: res?.page_size ?? query.pageSize ?? 20,
      totalCount: res?.total_count ?? 0,
      totalPages: res?.total_pages ?? 0,
    }
  },
  getProductById: async (id: string): Promise<Product | null> => {
    return fetchExternal<Product>(`/v1/store/products/${id}`)
  },
  getCategories: async (): Promise<Category[]> => {
    const res = await fetchExternal<Category[]>("/v1/store/categories")
    return res ?? []
  },
  // Current price/availability for specific products (cart re-validation).
  // Products the branch can't sell any more are simply missing.
  getProductsByIds: async (ids: string[]): Promise<Product[]> => {
    if (ids.length === 0) return []
    const res = await fetchExternal<PaginatedResponse<Product>>(
      `/v1/store/products?ids=${ids.map(encodeURIComponent).join(",")}`,
      { cache: "no-store" }
    )
    return res?.data ?? []
  },
  // null = the branch hasn't been set up for online orders yet.
  getBranch: async (): Promise<BranchInfo | null> => {
    return fetchExternal<BranchInfo>("/v1/store/branch", { cache: "no-store" })
  },
  placeOrder: async (
    request: PlaceOrderRequest,
    idempotencyKey: string
  ): Promise<PlaceOrderResult> => {
    const { status, body } = await requestExternal<
      Order & { error?: string; issues?: OrderIssue[] }
    >("/v1/store/orders", {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey, ...(await customerHeaders()) },
      body: JSON.stringify(request),
    })
    if (status === 200 || status === 201) return { ok: true, order: body }
    return {
      ok: false,
      status,
      error: body.error ?? "Something went wrong placing your order.",
      issues: body.issues ?? [],
    }
  },
  getOrder: async (id: string): Promise<Order | null> => {
    return fetchExternal<Order>(`/v1/store/orders/${encodeURIComponent(id)}`, {
      cache: "no-store",
      headers: await customerHeaders(),
    })
  },
  // Starts (or reuses the open) hosted payment page for an order awaiting
  // payment. 409 = not awaiting payment any more (paid, or cancelled).
  startPaymentSession: async (
    orderId: string,
    urls: { success_url: string; cancel_url: string }
  ) =>
    apiResult<{ redirect_url: string; expires_at: string }>(
      `/v1/store/orders/${encodeURIComponent(orderId)}/payment-session`,
      { method: "POST", body: JSON.stringify(urls) }
    ),
  respondToSubstitution: async (
    orderId: string,
    substitutionId: string,
    accept: boolean
  ) =>
    apiResult<Order>(
      `/v1/store/orders/${encodeURIComponent(orderId)}/substitutions/${encodeURIComponent(substitutionId)}`,
      {
        method: "POST",
        headers: await customerHeaders(),
        body: JSON.stringify({ accept }),
      }
    ),

  // --- customer accounts ---------------------------------------------------
  sendLoginCode: (email: string) =>
    apiResult<Record<string, never>>("/v1/store/auth/code", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),
  verifyLoginCode: (email: string, code: string, userAgent: string) =>
    apiResult<{ token: string; expires_at: string; customer: Customer }>(
      "/v1/store/auth/verify",
      {
        method: "POST",
        headers: { "User-Agent": userAgent },
        body: JSON.stringify({ email, code }),
      }
    ),
  logout: (token: string) =>
    requestExternal("/v1/store/auth/logout", {
      method: "POST",
      headers: { "X-Customer-Session": token },
    }),
  // null = not signed in (no cookie, or the session expired).
  me: async (token: string): Promise<Customer | null> => {
    const { status, body } = await requestExternal<Customer>("/v1/store/me", {
      headers: { "X-Customer-Session": token },
    })
    if (status === 401) return null
    if (status !== 200) throw new Error(`${status} loading customer`)
    return body
  },
  updateMe: async (changes: { name?: string; phone?: string }) =>
    apiResult<Customer>("/v1/store/me", {
      method: "PATCH",
      headers: await customerHeaders(),
      body: JSON.stringify(changes),
    }),
  myOrders: async (page: number) =>
    apiResult<{
      data: CustomerOrderSummary[]
      page: number
      page_size: number
      total_count: number
      total_pages: number
    }>(`/v1/store/me/orders?page=${page}&page_size=10`, {
      headers: await customerHeaders(),
    }),

  // --- push notifications --------------------------------------------------
  pushPublicKey: async (): Promise<string> => {
    const res = await fetchExternal<{ public_key: string }>(
      "/v1/store/push/config"
    ).catch(() => null)
    return res?.public_key ?? ""
  },
  savePushSubscription: async (
    subscription: PushSubscriptionJSON,
    orderId?: string
  ) =>
    apiResult<void>("/v1/store/push-subscriptions", {
      method: "POST",
      headers: await customerHeaders(),
      body: JSON.stringify({ subscription, order_id: orderId }),
    }),
  deletePushSubscription: (endpoint: string) =>
    apiResult<void>("/v1/store/push-subscriptions", {
      method: "DELETE",
      body: JSON.stringify({ endpoint }),
    }),

  // Opens the order's live stream (SSE), for the route handler that relays
  // it to the browser.
  openOrderStream: async (id: string, signal: AbortSignal) => {
    const baseUrl = (config.api.baseUrl ?? "").replace(/\/+$/, "")
    return fetch(
      `${baseUrl}/v1/store/orders/${encodeURIComponent(id)}/events`,
      {
        headers: {
          Authorization: `Bearer ${config.api.storefrontKey}`,
          Accept: "text/event-stream",
          ...(await customerHeaders()),
        },
        cache: "no-store",
        signal,
      }
    )
  },
}
