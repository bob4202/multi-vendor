import "server-only"

import { cache } from "react"
import { cookies } from "next/headers"
import { redirect } from "next/navigation"

import { adminAPI } from "@/lib/admin/api"
import { ADMIN_LOGIN_PATH, ADMIN_SESSION_COOKIE } from "@/lib/admin/constants"
import { ApiError } from "@/lib/api-client"
import type { PlatformAdmin } from "@/types/admin"

// Data access layer for the admin side: proxy.ts only checks that a cookie
// exists; this verifies the session against the API, once per request.

export const getAdmin = cache(async (): Promise<PlatformAdmin | null> => {
  const token = (await cookies()).get(ADMIN_SESSION_COOKIE)?.value
  if (!token) return null
  try {
    return await adminAPI.me(token)
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) return null
    throw err
  }
})

export async function requireAdmin(): Promise<PlatformAdmin> {
  const admin = await getAdmin()
  if (!admin) redirect(`${ADMIN_LOGIN_PATH}?expired=1`)
  return admin
}
