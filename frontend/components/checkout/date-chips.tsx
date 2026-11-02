"use client"

import { formatDateLabel } from "@/lib/format"
import type { OrderDay } from "@/lib/fulfilment"
import { cn } from "@/lib/utils"

interface DateChipsProps {
  days: OrderDay[]
  value: string
  onChange: (date: string) => void
  name?: string
  error?: string
}

// A row of selectable days ("Today", "Tue 13 Oct", ...). Scrolls sideways on
// narrow screens instead of wrapping into a tall block.
function DateChips({
  days,
  value,
  onChange,
  name = "fulfilment_date",
  error,
}: DateChipsProps) {
  return (
    <div className="flex flex-col gap-2">
      <div
        role="radiogroup"
        aria-invalid={!!error}
        className="-mx-1 flex [scrollbar-width:none] gap-2 overflow-x-auto px-1 pb-1"
      >
        {days.map((day) => {
          const selected = day.date === value
          return (
            <button
              key={day.date}
              type="button"
              role="radio"
              aria-checked={selected}
              onClick={() => onChange(day.date)}
              className={cn(
                "flex shrink-0 flex-col items-center rounded-lg border px-3 py-2 text-sm transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50",
                selected
                  ? "border-primary bg-primary text-primary-foreground"
                  : "hover:bg-muted"
              )}
            >
              <span className="font-medium">
                {day.isToday
                  ? "Today"
                  : formatDateLabel(day.date).split(" ")[0]}
              </span>
              <span
                className={cn(
                  "text-xs",
                  selected
                    ? "text-primary-foreground/80"
                    : "text-muted-foreground"
                )}
              >
                {formatDateLabel(day.date).split(" ").slice(1).join(" ")}
              </span>
            </button>
          )
        })}
      </div>
      <input type="hidden" name={name} value={value} />
      {error && <p className="text-xs text-destructive">{error}</p>}
    </div>
  )
}

export { DateChips }
