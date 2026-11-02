// GET /v1/store/branch - see README "Store info".
export interface OpeningHour {
  day: number // ISO: 1 = Monday ... 7 = Sunday
  open: string // "09:00"
  close: string // "20:00"
}

export interface Closure {
  date: string // "2026-12-25"
  closed: boolean
  open: string | null
  close: string | null
  note: string | null
}

export interface BranchInfo {
  name: string
  phone: string
  email: string | null
  timezone: string
  address: {
    line1: string | null
    line2: string | null
    city: string | null
    postcode: string | null
  }
  fulfilment: {
    collection: {
      enabled: boolean
      last_order_minutes: number // collection orders close this long before closing
    }
    delivery: {
      enabled: boolean
      fee: number // pence
      free_over: number | null // pence; null = no free-delivery threshold
      min_order: number // pence
      postcode_prefixes: string[]
    }
  }
  opening_hours: OpeningHour[]
  closures: Closure[]
  // Today's collection window; null = closed today or no collection.
  collection_today: {
    date: string
    last_order_at: string // ISO
    closes_at: string
    accepting: boolean // false once last orders have passed
  } | null
  // Card is the only way to pay: false = checkout is closed.
  payments: { enabled: boolean }
}
