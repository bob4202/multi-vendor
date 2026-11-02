// Shared by server modules and client components, so nothing server-only.

// httpOnly cookie holding the customer's session token. Path "/" because
// the whole storefront uses it (checkout prefill, order pages, account).
export const CUSTOMER_SESSION_COOKIE = "customer_session"

export const CUSTOMER_LOGIN_PATH = "/account/login"

// Version of the terms shown at /terms. Bump it whenever the terms change;
// every order records the version the customer accepted.
export const TERMS_VERSION = "2026-10-31"

// After signing in, only ever go back to a page on this site.
export function safeCustomerRedirect(next: string | null | undefined) {
  if (!next || !next.startsWith("/") || next.startsWith("//")) return "/account"
  return next
}
