"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"

// Re-fetches the current page's server data on an interval, so new orders
// appear without anyone reloading. Pauses while the tab is hidden and
// catches up as soon as it's visible again.
function AutoRefresh({ intervalMs = 30_000 }: { intervalMs?: number }) {
  const router = useRouter()

  useEffect(() => {
    const tick = () => {
      if (document.visibilityState === "visible") router.refresh()
    }
    const id = window.setInterval(tick, intervalMs)
    document.addEventListener("visibilitychange", tick)
    return () => {
      window.clearInterval(id)
      document.removeEventListener("visibilitychange", tick)
    }
  }, [router, intervalMs])

  return null
}

export { AutoRefresh }
