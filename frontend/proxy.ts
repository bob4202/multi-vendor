import { NextResponse, type NextRequest } from "next/server"

import {
  ADMIN_LOGIN_PATH,
  ADMIN_SESSION_COOKIE,
  adminPortalEnabled,
} from "@/lib/admin/constants"
import {
  VENDOR_LOGIN_PATH,
  VENDOR_SESSION_COOKIE,
} from "@/lib/vendor/constants"

// Optimistic auth checks for the signed-in portals: no session cookie, no
// page. They only look at the cookie - the real checks (is the session
// valid, can this user see this branch) happen in each portal's session.ts
// and in the API. The storefront never runs through here (see matcher).

interface Portal {
  prefix: string
  cookie: string
  loginPath: string
  // Paths reachable without a session (login, invite links).
  isPublic: (pathname: string) => boolean
}

const VENDOR: Portal = {
  prefix: "/vendor",
  cookie: VENDOR_SESSION_COOKIE,
  loginPath: VENDOR_LOGIN_PATH,
  isPublic: (p) => p === VENDOR_LOGIN_PATH || p.startsWith("/vendor/invite/"),
}

const ADMIN: Portal = {
  prefix: "/admin",
  cookie: ADMIN_SESSION_COOKIE,
  loginPath: ADMIN_LOGIN_PATH,
  isPublic: (p) => p === ADMIN_LOGIN_PATH,
}

export function proxy(request: NextRequest) {
  const { pathname, search } = request.nextUrl
  const isAdmin = pathname === "/admin" || pathname.startsWith("/admin/")

  // The admin side only exists on deployments that turn it on.
  if (isAdmin && !adminPortalEnabled()) {
    return new NextResponse("Not found", { status: 404 })
  }

  const portal = isAdmin ? ADMIN : VENDOR
  if (portal.isPublic(pathname) || request.cookies.has(portal.cookie)) {
    return NextResponse.next()
  }

  const login = new URL(portal.loginPath, request.url)
  login.searchParams.set("next", pathname + search)
  return NextResponse.redirect(login)
}

export const config = {
  matcher: ["/vendor", "/vendor/:path*", "/admin", "/admin/:path*"],
}
