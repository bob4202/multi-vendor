import type { PortalUser } from "@/components/portal/user-menu"
import type { VendorSession } from "@/types/vendor"

const ROLE_LABEL = { owner: "Owner", manager: "Manager", staff: "Staff" }

// How a signed-in vendor user appears in the portal's account menu.
export function vendorPortalUser(session: VendorSession): PortalUser {
  return {
    name: session.user.name,
    email: session.user.email,
    role: ROLE_LABEL[session.user.role],
    detail: session.vendor.name,
  }
}
