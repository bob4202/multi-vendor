// Server-only values - none of these are NEXT_PUBLIC_, so they never reach
// the browser. The vendor and admin portals sign people in with their own
// accounts, so the frontend never needs the backend's ADMIN_API_KEY.
export const config = {
  api: {
    baseUrl: process.env.BASE_URL,
    branchId: process.env.BRANCH_ID,
    storefrontKey: process.env.STOREFRONT_KEY,
  },
}
