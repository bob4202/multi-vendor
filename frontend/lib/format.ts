// Prices are stored in minor units (pence), e.g. 650 = £6.50.
export function formatPrice(pence: number | null) {
  if (pence === null) return null
  return new Intl.NumberFormat("en-GB", {
    style: "currency",
    currency: "GBP",
  }).format(pence / 100)
}

// For money <input>s: 650 -> "6.50", null -> "".
export function penceToPoundsInput(pence: number | null) {
  return pence === null ? "" : (pence / 100).toFixed(2)
}

// Parses what someone typed into a money input. "" -> null; anything that
// isn't a non-negative amount with at most 2 decimals -> NaN.
export function poundsInputToPence(value: string) {
  const v = value.trim().replace(/^£/, "")
  if (v === "") return null
  if (!/^\d+(\.\d{1,2})?$/.test(v)) return Number.NaN
  return Math.round(Number(v) * 100)
}

// Shops are in the UK; pinning the zone also keeps server and client
// renders identical (no hydration mismatch from the viewer's own zone).
const TIME_ZONE = "Europe/London"

// "2026-10-12" -> "Mon 12 Oct". Parsed as a calendar date (UTC midnight) and
// formatted in UTC, so the label can't drift a day in any timezone.
export function formatDateLabel(isoDate: string) {
  return new Intl.DateTimeFormat("en-GB", {
    weekday: "short",
    day: "numeric",
    month: "short",
    timeZone: "UTC",
  }).format(new Date(`${isoDate}T00:00:00Z`))
}

// ISO timestamp -> "14 Oct, 09:30" (UK time)
export function formatDateTime(timestamp: string) {
  return new Intl.DateTimeFormat("en-GB", {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: TIME_ZONE,
  }).format(new Date(timestamp))
}

// ISO timestamp -> "16:30" (UK time)
export function formatTime(timestamp: string) {
  return new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    timeZone: TIME_ZONE,
  }).format(new Date(timestamp))
}
