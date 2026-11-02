import "server-only"

import { cache } from "react"
import { cookies } from "next/headers"
import { notFound, redirect } from "next/navigation"

import { ApiError } from "@/lib/api-client"
import { vendorAPI } from "@/lib/vendor/api"
import {
  VENDOR_LOGIN_PATH,
  VENDOR_SESSION_COOKIE,
} from "@/lib/vendor/constants"
import type { VendorSession } from "@/types/vendor"

// Data access layer for the vendor portal (see Next's authentication guide).
// proxy.ts only checks that a cookie exists; this is where the session is
// actually verified, against the API, once per request.

export const getSession = cache(async (): Promise<VendorSession | null> => {
  const token = (await cookies()).get(VENDOR_SESSION_COOKIE)?.value
  if (!token) return null
  try {
    return await vendorAPI.me(token)
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) return null
    throw err
  }
})

export async function requireSession(): Promise<VendorSession> {
  const session = await getSession()
  if (!session) redirect(`${VENDOR_LOGIN_PATH}?expired=1`)
  return session
}

// The branch must be one this user can manage; anything else is a 404, the
// same answer the API gives, so branch IDs can't be probed.
export async function requireBranch(branchId: string) {
  const session = await requireSession()
  const branch = session.branches.find((b) => b.id === branchId)
  if (!branch) notFound()
  return { session, branch }
}
