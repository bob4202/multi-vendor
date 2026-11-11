import { Price } from "@/components/common/price"
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { PaymentStatusBadge } from "@/components/vendor/payment-status-badge"
import { RefundDialog } from "@/components/vendor/refund-dialog"
import { refundableAmount } from "@/lib/vendor/payment-status"
import type { OrderPayment } from "@/types/vendor"

// What happens next to the money, in the shop's words.
const NEXT: Partial<Record<OrderPayment["status"], string>> = {
  authorised: "Charged when you mark the order ready.",
  voided: "Nothing was charged.",
  capture_failed: "The card couldn't be charged.",
}

// Where the order's money is: held, charged, refunded, plus Refund for
// owners and managers once it's been taken.
function PaymentCard({
  payment,
  branchId,
  orderId,
  reference,
  canRefund,
}: {
  payment: OrderPayment
  branchId: string
  orderId: string
  reference: string
  canRefund: boolean
}) {
  const refundable = refundableAmount(payment)
  const rows = [
    { label: "Held on card", pence: payment.amount_authorised },
    { label: "Charged", pence: payment.amount_captured },
    { label: "Refunded", pence: payment.amount_refunded },
  ].filter((r) => r.pence > 0)

  return (
    <Card>
      <CardHeader>
        <CardTitle>Payment</CardTitle>
        <CardAction>
          <PaymentStatusBadge status={payment.status} />
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-3 text-sm">
        {rows.length > 0 && (
          <dl className="flex flex-col gap-1.5">
            {rows.map((r) => (
              <div key={r.label} className="flex justify-between">
                <dt className="text-muted-foreground">{r.label}</dt>
                <dd>
                  <Price pence={r.pence} />
                </dd>
              </div>
            ))}
          </dl>
        )}
        {NEXT[payment.status] && (
          <p className="text-xs text-muted-foreground">
            {NEXT[payment.status]}
          </p>
        )}
        {canRefund && refundable > 0 && (
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs text-muted-foreground">
              <Price pence={refundable} /> can be refunded
            </span>
            <RefundDialog
              branchId={branchId}
              orderId={orderId}
              reference={reference}
              refundable={refundable}
            />
          </div>
        )}
      </CardContent>
    </Card>
  )
}

export { PaymentCard }
