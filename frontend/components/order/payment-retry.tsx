"use client"

import { useState, useTransition } from "react"
import { CreditCard, Loader2 } from "lucide-react"

import { startPaymentAction } from "@/app/(storefront)/orders/actions"
import { Button } from "@/components/ui/button"
import { rememberPayingOrder } from "@/lib/cart"

// Opens the card payment page again for an order awaiting payment: after
// backing out of it, a declined card, or a page that timed out.
function PaymentRetry({ orderId, label }: { orderId: string; label: string }) {
  const [pending, startTransition] = useTransition()
  const [error, setError] = useState<string | null>(null)
  // Stays true while the browser leaves for the payment page.
  const [leaving, setLeaving] = useState(false)

  function pay() {
    setError(null)
    startTransition(async () => {
      const res = await startPaymentAction(orderId)
      if (!res.ok) {
        setError(res.error)
        return
      }
      setLeaving(true)
      rememberPayingOrder(orderId)
      window.location.assign(res.redirectUrl)
    })
  }

  const busy = pending || leaving
  return (
    <div className="flex flex-col items-center gap-2">
      <Button size="lg" onClick={pay} disabled={busy}>
        {busy ? <Loader2 className="animate-spin" /> : <CreditCard />}
        {busy ? "Opening payment…" : label}
      </Button>
      {error && (
        <p className="animate-in text-sm text-destructive fade-in-0">{error}</p>
      )}
    </div>
  )
}

export { PaymentRetry }
