import { CreditCard } from "lucide-react"

import { Price } from "@/components/common/price"

// Taking the money failed (hold expired, card closed): the shopping must
// not leave the shop. The API refuses the handover too; this says why.
function CaptureFailedAlert({ amount }: { amount: number }) {
  return (
    <div
      role="alert"
      className="mb-4 flex animate-in items-start gap-3 rounded-xl border border-destructive/40 bg-destructive/5 p-4 text-sm fade-in-0"
    >
      <CreditCard className="mt-0.5 size-4 shrink-0 text-destructive" />
      <div className="flex flex-col gap-1">
        <p className="font-medium text-destructive">
          Card couldn&apos;t be charged: don&apos;t hand over
        </p>
        <p className="text-muted-foreground">
          We couldn&apos;t take <Price pence={amount} /> from the
          customer&apos;s card, so the order can&apos;t be collected or sent
          out. Their order page tells them there was a problem with the payment.
        </p>
      </div>
    </div>
  )
}

export { CaptureFailedAlert }
