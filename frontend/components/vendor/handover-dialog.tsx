"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Loader2 } from "lucide-react"
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
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
} from "@/components/ui/input-otp"
import { Textarea } from "@/components/ui/textarea"
import { changeOrderStatusAction } from "@/lib/vendor/actions"

// Handing a collection order over: staff type the code the customer shows.
// Without it, they can still hand over with a reason (e.g. "checked ID"),
// which goes in the order's history.
function HandoverDialog({
  branchId,
  orderId,
  reference,
  open,
  onOpenChange,
  initialCode = "",
}: {
  branchId: string
  orderId: string
  reference: string
  open: boolean
  onOpenChange: (open: boolean) => void
  initialCode?: string
}) {
  const router = useRouter()
  const [code, setCode] = useState(initialCode)
  const [noCode, setNoCode] = useState(false)
  const [reason, setReason] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [pending, startTransition] = useTransition()

  function submit() {
    if (noCode && !reason.trim()) {
      setError("Say why you're handing over without the code.")
      return
    }
    if (!noCode && code.length !== 6) {
      setError("Enter the customer's 6-digit code.")
      return
    }
    startTransition(async () => {
      const res = await changeOrderStatusAction(
        branchId,
        orderId,
        "ready",
        "completed",
        undefined,
        noCode
          ? { code_override_reason: reason.trim() }
          : { collection_code: code }
      )
      if (res.ok) {
        toast.success(`${reference} handed over`)
        onOpenChange(false)
        return
      }
      if (res.status === 409) {
        toast.info("This order was just updated by someone else.")
        onOpenChange(false)
        router.refresh()
        return
      }
      setError(
        res.issues.find((i) => i.field === "collection_code")?.message ??
          res.error
      )
      setCode("")
    })
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !pending && onOpenChange(o)}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Hand over {reference}</DialogTitle>
          <DialogDescription>
            Ask the customer for the code on their order page.
          </DialogDescription>
        </DialogHeader>

        {noCode ? (
          <Field data-invalid={!!error || undefined}>
            <FieldLabel htmlFor="override-reason">
              Why hand over without the code?
            </FieldLabel>
            <Textarea
              id="override-reason"
              autoFocus
              maxLength={300}
              value={reason}
              placeholder="e.g. Checked their name and phone number"
              onChange={(e) => {
                setReason(e.target.value)
                setError(null)
              }}
              aria-invalid={!!error || undefined}
            />
            {error && <FieldError>{error}</FieldError>}
          </Field>
        ) : (
          <Field data-invalid={!!error || undefined}>
            <FieldLabel htmlFor="handover-code">Collection code</FieldLabel>
            <InputOTP
              id="handover-code"
              maxLength={6}
              inputMode="numeric"
              pattern="^[0-9]*$"
              autoFocus
              value={code}
              onChange={(v) => {
                setCode(v)
                setError(null)
              }}
              containerClassName="justify-center"
            >
              <InputOTPGroup>
                {Array.from({ length: 6 }, (_, i) => (
                  <InputOTPSlot
                    key={i}
                    index={i}
                    className="size-11 text-lg"
                    aria-invalid={!!error || undefined}
                  />
                ))}
              </InputOTPGroup>
            </InputOTP>
            {error && <FieldError>{error}</FieldError>}
          </Field>
        )}

        <Button
          variant="link"
          className="h-auto self-start p-0"
          onClick={() => {
            setNoCode(!noCode)
            setError(null)
          }}
        >
          {noCode ? "Enter the code instead" : "Customer doesn't have the code?"}
        </Button>

        <DialogFooter>
          <Button
            variant="outline"
            disabled={pending}
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button disabled={pending} onClick={submit}>
            {pending && <Loader2 className="animate-spin" />}
            Hand over
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export { HandoverDialog }
