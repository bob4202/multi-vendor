import { formatDateTime } from "@/lib/format"
import { ORDER_STATUS } from "@/lib/vendor/order-status"
import type { OrderEvent } from "@/types/vendor"

// History entries that aren't status changes keep the current status, so
// their kind decides the title.
const KIND_TITLE: Record<Exclude<OrderEvent["kind"], "status">, string> = {
  substitution: "Substitution",
  removed: "Item removed",
  code_override: "Handed over without the code",
  payment: "Payment",
}

// The order's audit trail: when it was placed, then every status change
// and item change, with who made it.
function OrderTimeline({
  createdAt,
  events,
}: {
  createdAt: string
  events: OrderEvent[]
}) {
  const entries = [
    {
      key: "placed",
      title: "Order placed",
      by: null,
      note: null,
      at: createdAt,
    },
    ...events.map((e, i) => ({
      key: String(i),
      title:
        !e.kind || e.kind === "status"
          ? ORDER_STATUS[e.to_status].label
          : KIND_TITLE[e.kind],
      by: e.actor_name,
      note: e.note,
      at: e.created_at,
    })),
  ]

  return (
    <ol className="flex flex-col">
      {entries.map((e, i) => (
        <li key={e.key} className="relative flex gap-3 pb-4 last:pb-0">
          {i < entries.length - 1 && (
            <span
              aria-hidden
              className="absolute top-3 left-[5px] h-full w-px bg-border"
            />
          )}
          <span
            aria-hidden
            className="relative mt-1.5 size-[11px] shrink-0 rounded-full border-2 border-primary bg-background"
          />
          <div className="flex min-w-0 flex-col gap-0.5">
            <span className="text-sm font-medium">{e.title}</span>
            <span className="text-xs text-muted-foreground">
              {formatDateTime(e.at)}
              {e.by && ` · ${e.by}`}
            </span>
            {e.note && (
              <p className="text-sm text-muted-foreground">“{e.note}”</p>
            )}
          </div>
        </li>
      ))}
    </ol>
  )
}

export { OrderTimeline }
