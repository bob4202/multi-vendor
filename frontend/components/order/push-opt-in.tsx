"use client"

import { useEffect, useState } from "react"
import { BellOff, BellRing, Loader2, Share } from "lucide-react"

import {
  deletePushSubscriptionAction,
  savePushSubscriptionAction,
} from "@/app/(storefront)/orders/actions"
import { Button } from "@/components/ui/button"
import type { PushSubscriptionJSON } from "@/lib/external-api-service"

type State =
  | "loading"
  | "unsupported"
  | "ios-install" // iPhone/iPad: push only works from the home screen
  | "denied"
  | "off"
  | "on"

function base64UrlToBytes(value: string) {
  const padded = (value + "=".repeat((4 - (value.length % 4)) % 4))
    .replace(/-/g, "+")
    .replace(/_/g, "/")
  const raw = atob(padded)
  return Uint8Array.from(raw, (c) => c.charCodeAt(0))
}

function isIOS() {
  return /iPad|iPhone|iPod/.test(navigator.userAgent)
}

function isStandalone() {
  return (
    window.matchMedia("(display-mode: standalone)").matches ||
    ("standalone" in navigator && Boolean(navigator.standalone))
  )
}

async function registration() {
  return navigator.serviceWorker.register("/sw.js", {
    scope: "/",
    updateViaCache: "none",
  })
}

// "Notify me" on the order page: a phone notification when the order is
// accepted, needs an answer, or is ready - even with the page closed.
function PushOptIn({
  orderId,
  publicKey,
}: {
  orderId: string
  publicKey: string
}) {
  const [state, setState] = useState<State>("loading")
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let cancelled = false
    async function check() {
      if (!("serviceWorker" in navigator) || !("PushManager" in window)) {
        setState(isIOS() && !isStandalone() ? "ios-install" : "unsupported")
        return
      }
      if (Notification.permission === "denied") {
        setState("denied")
        return
      }
      const reg = await registration()
      const sub = await reg.pushManager.getSubscription()
      if (cancelled) return
      if (sub && Notification.permission === "granted") {
        // Already on for this browser: make sure this order is linked too.
        await savePushSubscriptionAction(
          sub.toJSON() as PushSubscriptionJSON,
          orderId
        )
        setState("on")
      } else {
        setState("off")
      }
    }
    check().catch(() => !cancelled && setState("unsupported"))
    return () => {
      cancelled = true
    }
  }, [orderId])

  async function turnOn() {
    setBusy(true)
    try {
      const permission = await Notification.requestPermission()
      if (permission !== "granted") {
        setState(permission === "denied" ? "denied" : "off")
        return
      }
      const reg = await registration()
      const sub =
        (await reg.pushManager.getSubscription()) ??
        (await reg.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: base64UrlToBytes(publicKey),
        }))
      const saved = await savePushSubscriptionAction(
        sub.toJSON() as PushSubscriptionJSON,
        orderId
      )
      setState(saved ? "on" : "off")
    } catch {
      setState("off")
    } finally {
      setBusy(false)
    }
  }

  async function turnOff() {
    setBusy(true)
    try {
      const reg = await registration()
      const sub = await reg.pushManager.getSubscription()
      if (sub) {
        await deletePushSubscriptionAction(sub.endpoint)
        await sub.unsubscribe()
      }
      setState("off")
    } finally {
      setBusy(false)
    }
  }

  if (state === "loading" || state === "unsupported") return null

  return (
    <div className="flex animate-in flex-col gap-3 rounded-xl border p-4 duration-300 fade-in-0 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex items-start gap-3 text-sm">
        {state === "on" ? (
          <BellRing className="mt-0.5 size-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
        ) : state === "ios-install" ? (
          <Share className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        ) : (
          <BellOff className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        )}
        <p>
          {state === "on" &&
            "We'll send a notification when your order changes."}
          {state === "off" &&
            "Get a notification when your order is ready or the shop needs you."}
          {state === "denied" &&
            "Notifications are blocked for this site. Turn them on in your browser settings to get order updates."}
          {state === "ios-install" &&
            "For notifications on iPhone, tap Share, then “Add to Home Screen”, and open the shop from there."}
        </p>
      </div>
      {state === "off" && (
        <Button size="sm" onClick={turnOn} disabled={busy} className="shrink-0">
          {busy && <Loader2 className="animate-spin" />}
          Notify me
        </Button>
      )}
      {state === "on" && (
        <Button
          size="sm"
          variant="ghost"
          onClick={turnOff}
          disabled={busy}
          className="shrink-0"
        >
          Turn off
        </Button>
      )}
    </div>
  )
}

export { PushOptIn }
