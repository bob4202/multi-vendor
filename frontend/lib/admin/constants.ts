// Shared by proxy.ts and the server-only admin modules, so it must not
// import anything server-only itself.

export const ADMIN_SESSION_COOKIE = "admin_session"

export const ADMIN_LOGIN_PATH = "/admin/login"

// Every storefront deployment runs this same app, so the admin side is off
// unless a deployment explicitly turns it on. Set ADMIN_PORTAL=enabled only
// where admins should sign in (and locally).
export function adminPortalEnabled() {
  return process.env.ADMIN_PORTAL === "enabled"
}

// Only send people back to somewhere inside the admin side after login.
export function safeAdminRedirect(next: string | null | undefined) {
  if (!next || !next.startsWith("/admin") || next.startsWith("//")) {
    return "/admin"
  }
  return next
}
