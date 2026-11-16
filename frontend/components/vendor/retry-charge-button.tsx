"use client"

import { useTransition } from "react"
import { useRouter } from "next/navigation"
import { Loader2, RotateCw } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { retryChargeAction } from "@/lib/vendor/actions"

// Queues the capture again after a failed charge. The payment goes back to
// "Card held" straight away and the worker tries the card within a minute;
// if it fails again, the warning comes back.
function RetryChargeButton({
  branchId,
  orderId,
  reference,
}: {
  branchId: string
  orderId: string
  reference: string
}) {
  const router = useRouter()
  const [pending, startTransition] = useTransition()

  function retry() {
    startTransition(async () => {
      const res = await retryChargeAction(branchId, orderId)
      if (res.ok) {
        toast.success(`${reference}: trying the charge again`)
        return
      }
      if (res.status === 409) {
        // Already retried or cancelled by someone else: show what it is now.
        toast.info("This order was just updated by someone else.")
        router.refresh()
        return
      }
      toast.error(res.error)
    })
  }

  return (
    <Button variant="outline" size="sm" disabled={pending} onClick={retry}>
      {pending ? <Loader2 className="animate-spin" /> : <RotateCw />}
      Try charging again
    </Button>
  )
}

export { RetryChargeButton }
