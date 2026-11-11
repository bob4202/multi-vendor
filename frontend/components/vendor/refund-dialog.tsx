"use client"

import { useState, useTransition } from "react"
import { Loader2, Undo2 } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { refundOrderAction } from "@/lib/vendor/actions"
import {
  formatPrice,
  penceToPoundsInput,
  poundsInputToPence,
} from "@/lib/format"

type Errors = { amount?: string; reason?: string }

// "Refund" on a paid order: part or all of what's left. Owners and managers
// only (the page hides the button, the API enforces it). The worker gives
// the money back a few seconds later, and a payment line joins the history.
function RefundDialog({
  branchId,
  orderId,
  reference,
  refundable,
}: {
  branchId: string
  orderId: string
  reference: string
  refundable: number // pence still refundable
}) {
  const [open, setOpen] = useState(false)
  const [amount, setAmount] = useState("")
  const [reason, setReason] = useState("")
  const [errors, setErrors] = useState<Errors>({})
  const [pending, startTransition] = useTransition()

  function openDialog() {
    setAmount(penceToPoundsInput(refundable))
    setReason("")
    setErrors({})
    setOpen(true)
  }

  function validate(): { pence: number; note: string } | null {
    const pence = poundsInputToPence(amount)
    const note = reason.trim()
    const next: Errors = {}
    if (pence === null || Number.isNaN(pence)) {
      next.amount = "Enter an amount, like 4.50."
    } else if (pence <= 0) {
      next.amount = "Enter more than £0.00."
    } else if (pence > refundable) {
      next.amount = `You can refund up to ${formatPrice(refundable)}.`
    }
    if (!note) next.reason = "Say why - it's kept in the order history."
    setErrors(next)
    return next.amount || next.reason ? null : { pence: pence!, note }
  }

  function submit() {
    const valid = validate()
    if (!valid) return
    startTransition(async () => {
      const res = await refundOrderAction(
        branchId,
        orderId,
        valid.pence,
        valid.note
      )
      if (res.ok) {
        toast.success(
          `Refund of ${formatPrice(valid.pence)} on its way to the customer`
        )
        setOpen(false)
        return
      }
      if (res.status === 422) {
        const byField = (f: string) =>
          res.issues.find((i) => i.field === f)?.message
        setErrors({ amount: byField("amount"), reason: byField("reason") })
        if (byField("amount") || byField("reason")) return
      }
      toast.error(res.error.charAt(0).toUpperCase() + res.error.slice(1))
      if (res.status === 409) setOpen(false)
    })
  }

  return (
    <>
      <Button variant="outline" size="sm" onClick={openDialog}>
        <Undo2 />
        Refund
      </Button>

      <Dialog open={open} onOpenChange={(o) => !pending && setOpen(o)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Refund {reference}</DialogTitle>
            <DialogDescription>
              The money goes back to the customer&apos;s card. Up to{" "}
              {formatPrice(refundable)} can be refunded.
            </DialogDescription>
          </DialogHeader>

          <Field data-invalid={!!errors.amount || undefined}>
            <FieldLabel htmlFor="refund-amount">Amount</FieldLabel>
            <div className="flex items-center gap-2">
              <div className="relative w-36">
                <span className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-sm text-muted-foreground">
                  £
                </span>
                <Input
                  id="refund-amount"
                  inputMode="decimal"
                  autoComplete="off"
                  className="pl-6"
                  value={amount}
                  aria-invalid={!!errors.amount || undefined}
                  onChange={(e) => {
                    setAmount(e.target.value)
                    setErrors((er) => ({ ...er, amount: undefined }))
                  }}
                />
              </div>
              <Button
                variant="link"
                className="h-auto p-0"
                onClick={() => {
                  setAmount(penceToPoundsInput(refundable))
                  setErrors((er) => ({ ...er, amount: undefined }))
                }}
              >
                Full amount
              </Button>
            </div>
            {errors.amount && <FieldError>{errors.amount}</FieldError>}
          </Field>

          <Field data-invalid={!!errors.reason || undefined}>
            <FieldLabel htmlFor="refund-reason">Reason</FieldLabel>
            <Textarea
              id="refund-reason"
              value={reason}
              maxLength={500}
              placeholder="e.g. Eggs arrived broken"
              aria-invalid={!!errors.reason || undefined}
              onChange={(e) => {
                setReason(e.target.value)
                setErrors((er) => ({ ...er, reason: undefined }))
              }}
            />
            {errors.reason ? (
              <FieldError>{errors.reason}</FieldError>
            ) : (
              <FieldDescription>
                Kept in the order history with your name.
              </FieldDescription>
            )}
          </Field>

          <DialogFooter>
            <Button
              variant="outline"
              disabled={pending}
              onClick={() => setOpen(false)}
            >
              Cancel
            </Button>
            <Button disabled={pending} onClick={submit}>
              {pending && <Loader2 className="animate-spin" />}
              Refund {formatPrice(poundsInputToPence(amount) || 0)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

export { RefundDialog }
