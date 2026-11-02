import "server-only"

import { cookies, headers } from "next/headers"

import { ApiError } from "@/lib/api-client"

// Cookie and login helpers shared by the vendor and admin portals' server
// actions (a "use server" file can only export actions, so they live here).

// The cookie outlives the API's sliding idle timeout on purpose: the API is
// the authority on whether a session is still valid. 30 days matches the
// API's absolute session lifetime.
const SESSION_COOKIE_MAX_AGE = 30 * 24 * 60 * 60

// Each portal's cookie is scoped to its own path, so the vendor token is
// never even sent with admin requests and vice versa.
export async function setSessionCookie(
  name: string,
  path: string,
  token: string
) {
  ;(await cookies()).set(name, token, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path,
    maxAge: SESSION_COOKIE_MAX_AGE,
  })
}

// Returns the token that was set, so the caller can revoke it server-side.
export async function clearSessionCookie(name: string, path: string) {
  const store = await cookies()
  const token = store.get(name)?.value
  store.delete({ name, path })
  return token
}

// The browser's user agent, forwarded so sessions record the real device
// rather than the Next server.
export async function browserUserAgent() {
  return (await headers()).get("user-agent") ?? ""
}

export interface LoginState {
  error?: string
  email?: string
}

// Maps a failed login call to what the form shows. Unexpected errors get a
// generic message (and are logged), never details.
export function loginFailure(err: unknown, email: string): LoginState {
  if (err instanceof ApiError && err.status === 401) {
    return { error: "Incorrect email or password.", email }
  }
  if (err instanceof ApiError && err.status === 429) {
    return {
      error: "Too many failed attempts. Try again in 15 minutes.",
      email,
    }
  }
  console.error("login failed", err)
  return { error: "We couldn't sign you in. Please try again.", email }
}
