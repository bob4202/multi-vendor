"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Loader2 } from "lucide-react"
import { toast } from "sonner"

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { Textarea } from "@/components/ui/textarea"
import { HandoverDialog } from "@/components/vendor/handover-dialog"
import { changeOrderStatusAction } from "@/lib/vendor/actions"
import { ORDER_STATUS, reasonRequired } from "@/lib/vendor/order-status"
import type { OrderStatus } from "@/types/vendor"

// Buttons for the statuses this order can move to next (from the API's
// next_statuses, primary step first). Unhappy endings - reject, cancel,
// no-show - ask for confirmation and a reason first.
function OrderStatusActions({
  branchId,
  orderId,
  reference,
  status,
  nextStatuses,
  requiresCollectionCode = false,
  initialCode,
}: {
  branchId: string
  orderId: string
  reference: string
  status: OrderStatus
  nextStatuses: OrderStatus[]
  // Completing a collection order goes through the code check.
  requiresCollectionCode?: boolean
  // Code already typed in "Find by code" - opens the hand-over straight away.
  initialCode?: string
}) {
  const router = useRouter()
  const [pending, startTransition] = useTransition()
  const [confirming, setConfirming] = useState<OrderStatus | null>(null)
  const [reason, setReason] = useState("")
  const [reasonError, setReasonError] = useState<string | null>(null)
  const canHandOver =
    requiresCollectionCode && nextStatuses.includes("completed")
  const [handingOver, setHandingOver] = useState(
    canHandOver && !!initialCode
  )

  const forward = nextStatuses.filter((s) => !ORDER_STATUS[s].destructive)
  const endings = nextStatuses.filter((s) => ORDER_STATUS[s].destructive)

  function submit(next: OrderStatus, note?: string) {
    startTransition(async () => {
      const res = await changeOrderStatusAction(
        branchId,
        orderId,
        status,
        next,
        note
      )
      if (res.ok) {
        toast.success(`${reference}: ${ORDER_STATUS[next].label}`)
        setConfirming(null)
        return
      }
      if (res.status === 409) {
        // Someone else moved it first; show them what it is now.
        toast.info("This order was just updated by someone else.")
        setConfirming(null)
        router.refresh()
        return
      }
      toast.error(res.error)
    })
  }

  function confirmEnding() {
    if (!confirming) return
    const note = reason.trim()
    if (reasonRequired(confirming) && !note) {
      setReasonError("Tell the customer why - it's shown on their order.")
      return
    }
    submit(confirming, note || undefined)
  }

  if (nextStatuses.length === 0) return null

  return (
    <>
      {forward.map((s, i) => (
        <Button
          key={s}
          variant={i === 0 ? "default" : "outline"}
          size="lg"
          disabled={pending}
          onClick={() =>
            s === "completed" && canHandOver
              ? setHandingOver(true)
              : submit(s)
          }
        >
          {pending && i === 0 && <Loader2 className="animate-spin" />}
          {ORDER_STATUS[s].action}
        </Button>
      ))}
      {endings.map((s) => (
        <Button
          key={s}
          variant="destructive"
          size="lg"
          disabled={pending}
          onClick={() => {
            setReason("")
            setReasonError(null)
            setConfirming(s)
          }}
        >
          {ORDER_STATUS[s].action}
        </Button>
      ))}

      {canHandOver && (
        <HandoverDialog
          branchId={branchId}
          orderId={orderId}
          reference={reference}
          open={handingOver}
          onOpenChange={setHandingOver}
          initialCode={initialCode}
        />
      )}

      <AlertDialog
        open={confirming !== null}
        onOpenChange={(open) => !open && !pending && setConfirming(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirming && ORDER_STATUS[confirming].action} {reference}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              This ends the order and can&apos;t be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <Field data-invalid={!!reasonError || undefined}>
            <FieldLabel htmlFor="status-reason">
              Reason{" "}
              {confirming && !reasonRequired(confirming) && (
                <span className="font-normal text-muted-foreground">
                  (optional)
                </span>
              )}
            </FieldLabel>
            <Textarea
              id="status-reason"
              value={reason}
              maxLength={500}
              placeholder={
                confirming === "rejected"
                  ? "e.g. Lamb shanks are out of stock until Friday"
                  : undefined
              }
              aria-invalid={!!reasonError || undefined}
              onChange={(e) => {
                setReason(e.target.value)
                setReasonError(null)
              }}
            />
            {reasonError && <FieldError>{reasonError}</FieldError>}
          </Field>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={pending}>Keep order</AlertDialogCancel>
            <Button
              variant="destructive"
              disabled={pending}
              onClick={confirmEnding}
            >
              {pending && <Loader2 className="animate-spin" />}
              {confirming && ORDER_STATUS[confirming].action}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

export { OrderStatusActions }
