"use client"

import { useRef } from "react"
import { useRouter } from "next/navigation"
import { toast } from "sonner"

import { useLive } from "@/components/order/use-live"

// Keeps a vendor order screen current: reloads its data when any of the
// branch's orders change, and announces new orders with a toast and a
// short beep. Falls back to reloading every fallbackMs without the stream.
function LiveOrders({
  branchId,
  fallbackMs = 10_000,
}: {
  branchId: string
  fallbackMs?: number
}) {
  const router = useRouter()
  const pending = useRef<number | undefined>(undefined)

  // Several changes in a row (e.g. a substitution) cause one reload.
  const refreshSoon = () => {
    window.clearTimeout(pending.current)
    pending.current = window.setTimeout(() => router.refresh(), 300)
  }

  useLive<{ id: string; reference: string; op: string }>({
    url: `/vendor/${branchId}/events`,
    eventName: "order",
    onEvent: (e) => {
      if (e.op === "insert") {
        toast.info(`New order ${e.reference}`, {
          action: {
            label: "Open",
            onClick: () => router.push(`/vendor/${branchId}/orders/${e.id}`),
          },
        })
        beep()
      }
      refreshSoon()
    },
    poll: () => {
      if (document.visibilityState === "visible") router.refresh()
    },
    pollMs: fallbackMs,
  })

  return null
}

// A short two-tone chime. Browsers only allow sound after the page has been
// interacted with, so the first one may be silent.
function beep() {
  try {
    const ctx = new AudioContext()
    ;[880, 1320].forEach((freq, i) => {
      const osc = ctx.createOscillator()
      const gain = ctx.createGain()
      osc.frequency.value = freq
      gain.gain.setValueAtTime(0.15, ctx.currentTime + i * 0.18)
      gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + i * 0.18 + 0.16)
      osc.connect(gain).connect(ctx.destination)
      osc.start(ctx.currentTime + i * 0.18)
      osc.stop(ctx.currentTime + i * 0.18 + 0.17)
    })
  } catch {
    // no audio available
  }
}

export { LiveOrders }
