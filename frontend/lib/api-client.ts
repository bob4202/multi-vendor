import "server-only"

import { refresh } from "next/cache"
import { cookies } from "next/headers"
import { redirect } from "next/navigation"

import { config } from "@/lib/config"
import type { ActionResult, ApiIssue } from "@/types/api"

// Shared plumbing for the signed-in portals (vendor, admin): attaches the
// session token from its httpOnly cookie, sends an expired session back to
// login, and turns error responses into ApiError. Server-only, so tokens
// never reach the browser.

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public issues: ApiIssue[] = []
  ) {
    super(message)
  }
}

export interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE"
  body?: unknown
  // Defaults to the session cookie. Pass null for unauthenticated calls
  // (login, invites), which also get their 401s as errors, not redirects.
  token?: string | null
  headers?: HeadersInit
}

export function createApiClient({
  prefix,
  cookie,
  loginPath,
}: {
  prefix: string // e.g. "/v1/vendor"
  cookie: string
  loginPath: string
}) {
  async function request<T>(
    endpoint: string,
    { method = "GET", body, token, headers }: RequestOptions = {}
  ): Promise<T> {
    const baseUrl = (config.api.baseUrl ?? "").replace(/\/+$/, "")
    const sessionToken =
      token === undefined ? (await cookies()).get(cookie)?.value : token

    const res = await fetch(`${baseUrl}${prefix}${endpoint}`, {
      method,
      headers: {
        "Content-Type": "application/json",
        ...(sessionToken ? { Authorization: `Bearer ${sessionToken}` } : {}),
        ...headers,
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      cache: "no-store",
    })

    // An expired or revoked session mid-visit: back to login.
    if (res.status === 401 && token !== null) {
      redirect(`${loginPath}?expired=1`)
    }
    if (res.status === 204) {
      return undefined as T
    }

    const json = await res.json().catch(() => ({}))
    if (!res.ok) {
      throw new ApiError(
        res.status,
        json.error ?? res.statusText,
        json.issues ?? []
      )
    }
    return json as T
  }

  return { request }
}

// Turns a 404 into null - "not found" is an expected outcome for a lookup.
export async function orNull<T>(promise: Promise<T>): Promise<T | null> {
  try {
    return await promise
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return null
    throw err
  }
}

// Runs a server action's mutation: on success refreshes the page's data and
// returns it; an ApiError becomes something the form can show. Anything
// else - including the redirect() an expired session throws - propagates.
//
// refresh: false keeps the current page as it is - for results shown once
// (an invite link, a new key) that a re-render would otherwise replace.
export async function runAction<T>(
  fn: () => Promise<T>,
  { refresh: shouldRefresh = true }: { refresh?: boolean } = {}
): Promise<ActionResult<T>> {
  try {
    const data = await fn()
    if (shouldRefresh) refresh()
    return { ok: true, data }
  } catch (err) {
    if (err instanceof ApiError) {
      return {
        ok: false,
        error: err.message,
        status: err.status,
        issues: err.issues,
      }
    }
    throw err
  }
}
