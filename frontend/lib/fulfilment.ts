import type { BranchInfo } from "@/types/branch"

// Display-side copies of the backend's rules (internal/store/fulfillment.go).
// They only drive what the checkout *shows*; the backend re-checks all of
// it when the order is placed.

export interface OrderDay {
  date: string // "2026-10-12"
  isToday: boolean
}

// Today's date in the branch's timezone, as YYYY-MM-DD ("en-CA" formats
// dates that way).
function todayIn(timezone: string) {
  return new Intl.DateTimeFormat("en-CA", { timeZone: timezone }).format(
    new Date()
  )
}

// The next `count` days the shop is open, starting today (branch time).
// A closure for a date wins over the weekly hours, as on the backend.
export function upcomingOpenDays(
  branch: BranchInfo,
  count = 7,
  maxDaysAhead = 14
): OrderDay[] {
  const openWeekdays = new Set(branch.opening_hours.map((h) => h.day))
  const closures = new Map(branch.closures.map((c) => [c.date, c]))
  const today = todayIn(branch.timezone)
  const start = new Date(`${today}T00:00:00Z`)

  const days: OrderDay[] = []
  for (
    let offset = 0;
    offset <= maxDaysAhead && days.length < count;
    offset++
  ) {
    const d = new Date(start)
    d.setUTCDate(start.getUTCDate() + offset)
    const date = d.toISOString().slice(0, 10)
    const isoWeekday = d.getUTCDay() === 0 ? 7 : d.getUTCDay()

    const closure = closures.get(date)
    const open = closure ? !closure.closed : openWeekdays.has(isoWeekday)
    if (open) days.push({ date, isToday: offset === 0 })
  }
  return days
}

// "e15 1ng" -> "E15". Every UK inward code is 3 characters, so the outward
// code is everything before that.
export function outwardCode(postcode: string) {
  const compact = postcode.replace(/\s+/g, "").toUpperCase()
  return compact.length > 3 ? compact.slice(0, -3) : ""
}

// Shape check only (the backend has the full validation).
export function looksLikePostcode(postcode: string) {
  return /^[A-Z]{1,2}\d[A-Z\d]?\s*\d[A-Z]{2}$/i.test(postcode.trim())
}

export function deliversTo(branch: BranchInfo, postcode: string) {
  return branch.fulfilment.delivery.postcode_prefixes.includes(
    outwardCode(postcode)
  )
}

// Free once the subtotal reaches free_over ("free on orders of £X or more").
export function deliveryFee(branch: BranchInfo, subtotal: number) {
  const { fee, free_over } = branch.fulfilment.delivery
  if (free_over !== null && subtotal >= free_over) return 0
  return fee
}
