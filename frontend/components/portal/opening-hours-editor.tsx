"use client"

import { Plus, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { FieldError } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import type { OpeningHour } from "@/types/vendor"

export const WEEKDAYS = [
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
  "Sunday",
]

export interface Period {
  open: string
  close: string
}

// Hours grouped by ISO day (index 0 = Monday). An empty list = closed.
export type WeekHours = Period[][]

export function toWeekHours(hours: OpeningHour[]): WeekHours {
  const week: WeekHours = WEEKDAYS.map(() => [])
  for (const h of hours) week[h.day - 1]?.push({ open: h.open, close: h.close })
  return week
}

// Flattened in day order - the same order the API reports issue indexes
// against (opening_hours.<i>), see periodErrors.
export function fromWeekHours(week: WeekHours): OpeningHour[] {
  return week.flatMap((periods, i) =>
    periods.map((p) => ({ day: i + 1, open: p.open, close: p.close }))
  )
}

// Maps API issues like { field: "opening_hours.3" } back to a day/period.
export function periodErrors(
  week: WeekHours,
  fieldErrors: Record<string, string>
): Record<string, string> {
  const out: Record<string, string> = {}
  let flat = 0
  week.forEach((periods, day) =>
    periods.forEach((_, p) => {
      const msg = fieldErrors[`opening_hours.${flat++}`]
      if (msg) out[`${day}.${p}`] = msg
    })
  )
  return out
}

function OpeningHoursEditor({
  value,
  onChange,
  errors,
  disabled,
}: {
  value: WeekHours
  onChange: (week: WeekHours) => void
  errors: Record<string, string> // keyed "day.period", see periodErrors
  disabled?: boolean
}) {
  const setDay = (day: number, periods: Period[]) =>
    onChange(value.map((p, i) => (i === day ? periods : p)))

  return (
    <div className="flex flex-col divide-y">
      {WEEKDAYS.map((name, day) => {
        const periods = value[day]
        const open = periods.length > 0
        return (
          <div
            key={name}
            className="grid grid-cols-1 gap-2 py-3 first:pt-0 last:pb-0 sm:grid-cols-[9rem_minmax(0,1fr)]"
          >
            <label className="flex items-center gap-2.5 text-sm font-medium sm:h-8">
              <Switch
                checked={open}
                disabled={disabled}
                onCheckedChange={(checked) =>
                  setDay(
                    day,
                    checked ? [{ open: "09:00", close: "18:00" }] : []
                  )
                }
                aria-label={`Open on ${name}`}
              />
              {name}
            </label>

            {open ? (
              <div className="flex flex-col gap-2">
                {periods.map((period, p) => {
                  const error = errors[`${day}.${p}`]
                  const update = (patch: Partial<Period>) =>
                    setDay(
                      day,
                      periods.map((x, j) => (j === p ? { ...x, ...patch } : x))
                    )
                  return (
                    <div key={p} className="flex flex-col gap-1">
                      <div className="flex items-center gap-2">
                        <Input
                          type="time"
                          value={period.open}
                          disabled={disabled}
                          aria-label={`${name} opens`}
                          aria-invalid={!!error || undefined}
                          onChange={(e) => update({ open: e.target.value })}
                          className="w-32"
                        />
                        <span className="text-sm text-muted-foreground">
                          to
                        </span>
                        <Input
                          type="time"
                          value={period.close}
                          disabled={disabled}
                          aria-label={`${name} closes`}
                          aria-invalid={!!error || undefined}
                          onChange={(e) => update({ close: e.target.value })}
                          className="w-32"
                        />
                        {periods.length > 1 && !disabled && (
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon-sm"
                            aria-label="Remove these hours"
                            onClick={() =>
                              setDay(
                                day,
                                periods.filter((_, j) => j !== p)
                              )
                            }
                          >
                            <X />
                          </Button>
                        )}
                      </div>
                      {error && <FieldError>{error}</FieldError>}
                    </div>
                  )
                })}
                {!disabled && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="w-fit"
                    onClick={() =>
                      setDay(day, [
                        ...periods,
                        { open: "14:00", close: "18:00" },
                      ])
                    }
                  >
                    <Plus />
                    Add hours
                  </Button>
                )}
              </div>
            ) : (
              <span className="text-sm text-muted-foreground sm:leading-8">
                Closed
              </span>
            )}
          </div>
        )
      })}
    </div>
  )
}

export { OpeningHoursEditor }
