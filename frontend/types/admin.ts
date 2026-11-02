// Shapes returned by the platform admin API (/v1/admin/*).

import type { VendorRole } from "@/types/vendor"

export interface PlatformAdmin {
  id: string
  name: string
  email: string
}

export type AttentionKind =
  | "sync_failed"
  | "account_locked"
  | "vendor_without_owner"
  | "invite_expired"
  | "invite_pending"
  | "no_store_details"
  | "no_storefront_key"
  | "never_synced"

export interface AttentionItem {
  kind: AttentionKind
  vendor_id: string
  vendor_name: string
  branch_id: string | null
  branch_name: string | null
  detail: string
  since: string | null
}

export interface ActivityItem {
  action: string
  target_type: string
  target_id: string
  summary: string
  admin_name: string | null // null = done with the shared ADMIN_API_KEY
  created_at: string
}

export interface PlatformOverview {
  counts: {
    active_vendors: number
    active_branches: number
    live_branches: number
    pending_invites: number
    failed_syncs: number
  }
  attention: AttentionItem[]
  recent_activity: ActivityItem[]
}

export interface AdminVendor {
  id: string
  slug: string
  name: string
  is_active: boolean
}

export interface AdminVendorListItem extends AdminVendor {
  branch_count: number
  user_count: number
  pending_invites: number
}

export interface AdminVendorBranch {
  id: string
  name: string
  host: string
  is_active: boolean
  storefront_api_key_prefix: string | null
  has_store_details: boolean
  last_sync_at: string | null
  last_sync_status: "success" | "error" | null
  last_sync_error: string | null
}

export type AdminVendorUserStatus =
  "active" | "invited" | "invite_expired" | "locked" | "disabled"

export interface AdminVendorUser {
  id: string
  name: string
  email: string
  role: VendorRole
  status: AdminVendorUserStatus
  last_login_at: string | null
  invite_expires_at: string | null
}

export interface AdminVendorDetail extends AdminVendor {
  branches: AdminVendorBranch[]
  users: AdminVendorUser[]
}

// A branch's inventory system connection settings (the API key is write-only).
export interface AdminBranch {
  id: string
  name: string
  host: string
  api_username: string
  is_active: boolean
  allowed_secondary_department_uuids: string[]
  allowed_product_refs: string[]
  storefront_api_key_prefix: string | null
  vendor_id: string
  storefront_url: string | null // base URL for links in customer emails
}

export interface BranchInput {
  name: string
  host: string
  api_username: string
  api_key?: string // required on create; omit on update to keep the current one
  is_active?: boolean
  allowed_secondary_department_uuids: string[]
  vendor_id?: string
  storefront_url?: string // "" clears it
}

export interface Invite {
  token: string
  expires_at: string
}

export interface StorefrontKey {
  api_key: string
  prefix: string
}
