"use server"

import { redirect } from "next/navigation"

import { ApiError, runAction } from "@/lib/api-client"
import {
  browserUserAgent,
  clearSessionCookie,
  loginFailure,
  setSessionCookie,
  type LoginState,
} from "@/lib/portal-session"
import { vendorAPI } from "@/lib/vendor/api"
import {
  safeVendorRedirect,
  VENDOR_LOGIN_PATH,
  VENDOR_SESSION_COOKIE,
} from "@/lib/vendor/constants"
import type { Closure, OrderStatus, StoreInfo } from "@/types/vendor"

// Server actions for the vendor portal. Every call goes through the API
// with the session cookie, and the API checks the session, branch access
// and role - so these stay thin and never decide permissions themselves.

const COOKIE_PATH = "/vendor"

export async function loginAction(
  _prev: LoginState,
  formData: FormData
): Promise<LoginState> {
  const email = String(formData.get("email") ?? "").trim()
  const password = String(formData.get("password") ?? "")
  if (!email || !password) {
    return { error: "Enter your email and password.", email }
  }

  try {
    const { token } = await vendorAPI.login(
      email,
      password,
      await browserUserAgent()
    )
    await setSessionCookie(VENDOR_SESSION_COOKIE, COOKIE_PATH, token)
  } catch (err) {
    return loginFailure(err, email)
  }
  redirect(safeVendorRedirect(String(formData.get("next") ?? "")))
}

export async function logoutAction() {
  const token = await clearSessionCookie(VENDOR_SESSION_COOKIE, COOKIE_PATH)
  if (token) {
    // Revoke server-side too, so the token is dead even if it leaked.
    await vendorAPI.logout(token).catch(() => {})
  }
  redirect(VENDOR_LOGIN_PATH)
}

export interface AcceptInviteState {
  error?: string
}

// Sets the invited user's password and signs them straight in.
export async function acceptInviteAction(
  token: string,
  _prev: AcceptInviteState,
  formData: FormData
): Promise<AcceptInviteState> {
  const password = String(formData.get("password") ?? "")
  if (password !== String(formData.get("confirm") ?? "")) {
    return { error: "The passwords don't match." }
  }

  try {
    const session = await vendorAPI.acceptInvite(
      token,
      password,
      await browserUserAgent()
    )
    await setSessionCookie(VENDOR_SESSION_COOKIE, COOKIE_PATH, session.token)
  } catch (err) {
    if (err instanceof ApiError && err.status === 422) {
      return { error: err.issues[0]?.message ?? err.message }
    }
    if (err instanceof ApiError && (err.status === 410 || err.status === 404)) {
      return {
        error: "This link has expired or was already used. Ask for a new one.",
      }
    }
    console.error("accept invite failed", err)
    return { error: "Something went wrong. Please try again." }
  }
  redirect("/vendor")
}

export async function changeOrderStatusAction(
  branchId: string,
  orderId: string,
  expectedStatus: OrderStatus,
  status: OrderStatus,
  reason?: string,
  handover?: { collection_code?: string; code_override_reason?: string }
) {
  return runAction(async () => {
    await vendorAPI.changeOrderStatus(branchId, orderId, {
      status,
      expected_status: expectedStatus,
      reason,
      ...handover,
    })
  })
}

export async function markItemUnavailableAction(
  branchId: string,
  orderId: string,
  itemId: string,
  substitute: { product_id: string; quantity: number } | null
) {
  return runAction(async () => {
    await vendorAPI.markItemUnavailable(branchId, orderId, itemId, substitute)
  })
}

export async function withdrawSubstitutionAction(
  branchId: string,
  orderId: string,
  itemId: string
) {
  return runAction(async () => {
    await vendorAPI.withdrawSubstitution(branchId, orderId, itemId)
  })
}

// Product search for the substitute picker (first page of matches).
export async function searchProductsAction(branchId: string, search: string) {
  return runAction(
    async () => (await vendorAPI.listProducts(branchId, { search, page: 1 })).data,
    { refresh: false }
  )
}

// Staff typing the code a customer shows at the counter.
export async function findOrderByCodeAction(branchId: string, code: string) {
  return runAction(
    async () => (await vendorAPI.lookupOrderByCode(branchId, code))?.data ?? [],
    { refresh: false }
  )
}

export async function saveStoreInfoAction(branchId: string, info: StoreInfo) {
  return runAction(() => vendorAPI.saveStoreInfo(branchId, info))
}

export async function addClosureAction(
  branchId: string,
  closure: Omit<Closure, "id">
) {
  return runAction(() => vendorAPI.addClosure(branchId, closure))
}

export async function deleteClosureAction(branchId: string, closureId: number) {
  return runAction(() => vendorAPI.deleteClosure(branchId, closureId))
}

export async function triggerSyncAction(branchId: string) {
  return runAction(async () => {
    await vendorAPI.triggerSync(branchId)
  })
}
