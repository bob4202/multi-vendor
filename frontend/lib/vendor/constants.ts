// Shared by proxy.ts and the server-only vendor modules, so it must not
// import anything server-only itself.

export const VENDOR_SESSION_COOKIE = "vendor_session"

export const VENDOR_LOGIN_PATH = "/vendor/login"

// Only send people back to somewhere inside the portal after login - never
// an arbitrary URL from the query string (open redirect).
export function safeVendorRedirect(next: string | null | undefined) {
  if (!next || !next.startsWith("/vendor") || next.startsWith("//")) {
    return "/vendor"
  }
  return next
}

// The API answers 409 for several reasons on a status change. Only this one
// means "someone else moved the order first"; the others (card couldn't be
// charged, accept window closed) carry a message the shop must see.
const STALE_STATUS_ERROR = "order status has changed since it was loaded"

export function isStaleStatus(res: { status?: number; error: string }) {
  return res.status === 409 && res.error === STALE_STATUS_ERROR
}
