"use client"

import { useState, useTransition } from "react"
import { CalendarOff, Loader2, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Field,
  FieldContent,
  FieldError,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { formatDateLabel } from "@/lib/format"
import { addClosureAction, deleteClosureAction } from "@/lib/vendor/actions"
import type { Closure } from "@/types/vendor"

const emptyForm = {
  date: "",
  closed: true,
  open: "10:00",
  close: "14:00",
  note: "",
}

// Holidays and one-off hours (Eid, Christmas...). Each add/remove saves
// straight away - there's nothing to batch.
function ClosuresEditor({
  branchId,
  closures,
  readOnly,
}: {
  branchId: string
  closures: Closure[]
  readOnly: boolean
}) {
  const [form, setForm] = useState(emptyForm)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [pending, startTransition] = useTransition()
  const [deleting, setDeleting] = useState<number | null>(null)

  function add(e: React.FormEvent) {
    e.preventDefault()
    if (!form.date) {
      setErrors({ date: "Pick a date" })
      return
    }
    startTransition(async () => {
      const res = await addClosureAction(branchId, {
        date: form.date,
        closed: form.closed,
        open: form.closed ? null : form.open,
        close: form.closed ? null : form.close,
        note: form.note.trim() || null,
      })
      if (res.ok) {
        setForm(emptyForm)
        setErrors({})
        toast.success(`${formatDateLabel(res.data.date)} added`)
        return
      }
      if (res.issues.length) {
        setErrors(
          Object.fromEntries(res.issues.map((i) => [i.field, i.message]))
        )
      } else {
        setErrors({ date: res.error })
      }
    })
  }

  function remove(c: Closure) {
    setDeleting(c.id)
    startTransition(async () => {
      const res = await deleteClosureAction(branchId, c.id)
      setDeleting(null)
      if (res.ok) toast.success(`${formatDateLabel(c.date)} removed`)
      else toast.error(res.error)
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Holidays and special hours</CardTitle>
        <CardDescription>
          These override your usual week for one day. Customers can&apos;t pick
          a closed day for collection or delivery.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {closures.length === 0 ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <CalendarOff className="size-4" />
            No upcoming changes to your hours.
          </p>
        ) : (
          <ul className="flex flex-col divide-y">
            {closures.map((c) => (
              <li
                key={c.id}
                className="flex items-center gap-3 py-2 first:pt-0"
              >
                <div className="flex min-w-0 flex-1 flex-col">
                  <span className="text-sm font-medium">
                    {formatDateLabel(c.date)}
                  </span>
                  <span className="truncate text-xs text-muted-foreground">
                    {c.closed ? "Closed all day" : `Open ${c.open}–${c.close}`}
                    {c.note && ` · ${c.note}`}
                  </span>
                </div>
                {!readOnly && (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Remove ${formatDateLabel(c.date)}`}
                    disabled={pending}
                    onClick={() => remove(c)}
                  >
                    {deleting === c.id ? (
                      <Loader2 className="animate-spin" />
                    ) : (
                      <Trash2 />
                    )}
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}

        {!readOnly && (
          <>
            <Separator />
            <form onSubmit={add} className="flex flex-col gap-4">
              <fieldset disabled={pending} className="contents">
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <Field data-invalid={!!errors.date || undefined}>
                    <FieldLabel htmlFor="closure-date">Date</FieldLabel>
                    <Input
                      id="closure-date"
                      type="date"
                      value={form.date}
                      aria-invalid={!!errors.date || undefined}
                      onChange={(e) =>
                        setForm({ ...form, date: e.target.value })
                      }
                    />
                    {errors.date && <FieldError>{errors.date}</FieldError>}
                  </Field>
                  <Field data-invalid={!!errors.note || undefined}>
                    <FieldLabel htmlFor="closure-note">Note</FieldLabel>
                    <Input
                      id="closure-note"
                      value={form.note}
                      maxLength={200}
                      placeholder="e.g. Eid al-Fitr"
                      onChange={(e) =>
                        setForm({ ...form, note: e.target.value })
                      }
                    />
                    {errors.note && <FieldError>{errors.note}</FieldError>}
                  </Field>
                </div>
                <Field orientation="horizontal">
                  <Switch
                    id="closure-closed"
                    checked={form.closed}
                    onCheckedChange={(closed) => setForm({ ...form, closed })}
                  />
                  <FieldContent>
                    <FieldLabel htmlFor="closure-closed">
                      Closed all day
                    </FieldLabel>
                  </FieldContent>
                </Field>
                {!form.closed && (
                  <Field
                    data-invalid={!!(errors.open || errors.close) || undefined}
                  >
                    <FieldLabel>Special hours</FieldLabel>
                    <div className="flex items-center gap-2">
                      <Input
                        type="time"
                        aria-label="Opens"
                        className="w-32"
                        value={form.open}
                        onChange={(e) =>
                          setForm({ ...form, open: e.target.value })
                        }
                      />
                      <span className="text-sm text-muted-foreground">to</span>
                      <Input
                        type="time"
                        aria-label="Closes"
                        className="w-32"
                        value={form.close}
                        onChange={(e) =>
                          setForm({ ...form, close: e.target.value })
                        }
                      />
                    </div>
                    {(errors.open || errors.close) && (
                      <FieldError>{errors.open ?? errors.close}</FieldError>
                    )}
                  </Field>
                )}
                <Button type="submit" variant="outline" className="w-fit">
                  {pending && deleting === null && (
                    <Loader2 className="animate-spin" />
                  )}
                  Add date
                </Button>
              </fieldset>
            </form>
          </>
        )}
      </CardContent>
    </Card>
  )
}

export { ClosuresEditor }
