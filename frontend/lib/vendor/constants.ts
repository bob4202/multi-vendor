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
