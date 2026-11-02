import "server-only"

import { createApiClient, orNull } from "@/lib/api-client"
import { ADMIN_LOGIN_PATH, ADMIN_SESSION_COOKIE } from "@/lib/admin/constants"
import type {
  AdminBranch,
  AdminVendor,
  AdminVendorDetail,
  AdminVendorListItem,
  BranchInput,
  Invite,
  PlatformAdmin,
  PlatformOverview,
  StorefrontKey,
} from "@/types/admin"
import type { Category } from "@/types/category"
import type {
  StoreInfo,
  SyncState,
  VendorRole,
  VendorUser,
} from "@/types/vendor"

// Platform admin API client (/v1/admin). Server-only: the admin session
// token lives in an httpOnly cookie and is attached by createApiClient.

const { request } = createApiClient({
  prefix: "/v1/admin",
  cookie: ADMIN_SESSION_COOKIE,
  loginPath: ADMIN_LOGIN_PATH,
})

const id = (value: string) => encodeURIComponent(value)

export const adminAPI = {
  login: (email: string, password: string, userAgent: string) =>
    request<{ token: string; expires_at: string; admin: PlatformAdmin }>(
      "/auth/login",
      {
        method: "POST",
        body: { email, password },
        token: null,
        headers: { "User-Agent": userAgent },
      }
    ),

  logout: (token: string) =>
    request<void>("/auth/logout", { method: "POST", token }),

  me: (token: string) => request<PlatformAdmin>("/me", { token }),

  overview: () => request<PlatformOverview>("/overview"),

  listVendors: (search?: string) =>
    request<AdminVendorListItem[]>(
      `/vendors${search ? `?q=${encodeURIComponent(search)}` : ""}`
    ),

  getVendor: (vendorId: string) =>
    orNull(request<AdminVendorDetail>(`/vendors/${id(vendorId)}`)),

  createVendor: (body: { slug: string; name: string }) =>
    request<AdminVendor>("/vendors", { method: "POST", body }),

  updateVendor: (
    vendorId: string,
    body: { name?: string; is_active?: boolean }
  ) =>
    request<AdminVendor>(`/vendors/${id(vendorId)}`, {
      method: "PATCH",
      body,
    }),

  inviteUser: (
    vendorId: string,
    body: {
      email: string
      name: string
      role: VendorRole
      branch_ids?: string[]
    }
  ) =>
    request<{ user: VendorUser; invite: Invite | null }>(
      `/vendors/${id(vendorId)}/users`,
      { method: "POST", body }
    ),

  reissueInvite: (vendorId: string, userId: string) =>
    request<Invite>(`/vendors/${id(vendorId)}/users/${id(userId)}/invite`, {
      method: "POST",
    }),

  // Inventory systems branches can sync from on this server.
  listInventoryProviders: async () =>
    (await request<{ providers: string[] }>("/inventory-providers")).providers,

  getBranch: (branchId: string) =>
    orNull(request<AdminBranch>(`/branches/${id(branchId)}`)),

  createBranch: (body: BranchInput) =>
    request<AdminBranch>("/branches", { method: "POST", body }),

  updateBranch: (branchId: string, body: Partial<BranchInput>) =>
    request<AdminBranch>(`/branches/${id(branchId)}`, {
      method: "PUT",
      body,
    }),

  getStoreInfo: (branchId: string) =>
    request<StoreInfo>(`/branches/${id(branchId)}/info`),

  saveStoreInfo: (branchId: string, info: StoreInfo) =>
    request<StoreInfo>(`/branches/${id(branchId)}/info`, {
      method: "PUT",
      body: info,
    }),

  issueStorefrontKey: (branchId: string) =>
    request<StorefrontKey>(`/branches/${id(branchId)}/storefront-key`, {
      method: "POST",
    }),

  triggerSync: (branchId: string) =>
    request<{ status: string }>(`/branches/${id(branchId)}/sync`, {
      method: "POST",
    }),

  // The API answers null (not []) for a branch that has never synced.
  syncState: async (branchId: string) =>
    (await request<SyncState[] | null>(
      `/branches/${id(branchId)}/sync-state`
    )) ?? [],

  listCategories: () => request<Category[]>("/categories"),
}
