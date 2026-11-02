// A signed-in storefront customer (GET /v1/store/me). Accounts belong to
// the vendor, so one email can have separate accounts at different shops.
export interface Customer {
  id: string
  email: string
  name: string | null
  phone: string | null // E.164
}
