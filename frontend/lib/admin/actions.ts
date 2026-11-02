"use server"

import { headers } from "next/headers"
import { redirect } from "next/navigation"

import { adminAPI } from "@/lib/admin/api"
import {
  ADMIN_LOGIN_PATH,
  ADMIN_SESSION_COOKIE,
  safeAdminRedirect,
} from "@/lib/admin/constants"
import { runAction } from "@/lib/api-client"
import {
  browserUserAgent,
  clearSessionCookie,
  loginFailure,
  setSessionCookie,
  type LoginState,
} from "@/lib/portal-session"
import type { BranchInput, Invite } from "@/types/admin"
import type { StoreInfo, VendorRole } from "@/types/vendor"

// Server actions for the admin side. The API checks the admin session on
// every call; these only shape requests and results for the forms.

const COOKIE_PATH = "/admin"

export async function adminLoginAction(
  _prev: LoginState,
  formData: FormData
): Promise<LoginState> {
  const email = String(formData.get("email") ?? "").trim()
  const password = String(formData.get("password") ?? "")
  if (!email || !password) {
    return { error: "Enter your email and password.", email }
  }

  try {
    const { token } = await adminAPI.login(
      email,
      password,
      await browserUserAgent()
    )
    await setSessionCookie(ADMIN_SESSION_COOKIE, COOKIE_PATH, token)
  } catch (err) {
    return loginFailure(err, email)
  }
  redirect(safeAdminRedirect(String(formData.get("next") ?? "")))
}

export async function adminLogoutAction() {
  const token = await clearSessionCookie(ADMIN_SESSION_COOKIE, COOKIE_PATH)
  if (token) {
    await adminAPI.logout(token).catch(() => {})
  }
  redirect(ADMIN_LOGIN_PATH)
}

export async function createVendorAction(input: {
  name: string
  slug: string
}) {
  return runAction(() => adminAPI.createVendor(input))
}

export async function updateVendorAction(
  vendorId: string,
  body: { name?: string; is_active?: boolean }
) {
  return runAction(() => adminAPI.updateVendor(vendorId, body))
}

// Creates the branch, or updates it when branchId is given. An empty
// api_key on update keeps the current one.
export async function saveBranchAction(
  vendorId: string,
  branchId: string | null,
  input: BranchInput
) {
  return runAction(() =>
    branchId
      ? adminAPI.updateBranch(branchId, {
          ...input,
          api_key: input.api_key || undefined,
        })
      : adminAPI.createBranch({ ...input, vendor_id: vendorId })
  )
}

export async function saveBranchStoreInfoAction(
  branchId: string,
  info: StoreInfo
) {
  return runAction(() => adminAPI.saveStoreInfo(branchId, info))
}

export interface InviteLink {
  url: string
  expires_at: string
}

// Invite links point at the vendor portal. Any deployment serves it, so by
// default use the host the admin is on; VENDOR_PORTAL_URL overrides that
// (e.g. a dedicated vendors.example.com).
async function inviteLink(invite: Invite): Promise<InviteLink> {
  let origin = process.env.VENDOR_PORTAL_URL?.replace(/\/+$/, "")
  if (!origin) {
    const h = await headers()
    const host = h.get("x-forwarded-host") ?? h.get("host") ?? "localhost:3000"
    const proto =
      h.get("x-forwarded-proto") ??
      (host.startsWith("localhost") ? "http" : "https")
    origin = `${proto}://${host}`
  }
  return {
    url: `${origin}/vendor/invite/${invite.token}`,
    expires_at: invite.expires_at,
  }
}

// No refresh: the page must keep showing the link until it's been copied.
export async function inviteUserAction(
  vendorId: string,
  input: {
    name: string
    email: string
    role: VendorRole
    branch_ids?: string[]
  }
) {
  return runAction(
    async () => {
      const { invite } = await adminAPI.inviteUser(vendorId, input)
      if (!invite)
        throw new Error("expected an invite for a user without a password")
      return inviteLink(invite)
    },
    { refresh: false }
  )
}

export async function reissueInviteAction(vendorId: string, userId: string) {
  return runAction(
    async () => inviteLink(await adminAPI.reissueInvite(vendorId, userId)),
    { refresh: false }
  )
}

// No refresh: the key is shown exactly once.
export async function issueStorefrontKeyAction(branchId: string) {
  return runAction(() => adminAPI.issueStorefrontKey(branchId), {
    refresh: false,
  })
}

export async function triggerBranchSyncAction(branchId: string) {
  return runAction(async () => {
    await adminAPI.triggerSync(branchId)
  })
}
