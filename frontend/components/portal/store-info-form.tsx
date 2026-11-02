"use client"

import { useState, useTransition } from "react"
import { Loader2 } from "lucide-react"
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
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import {
  OpeningHoursEditor,
  fromWeekHours,
  periodErrors,
  toWeekHours,
  type WeekHours,
} from "@/components/portal/opening-hours-editor"
import { penceToPoundsInput, poundsInputToPence } from "@/lib/format"
import type { ActionResult } from "@/types/api"
import type { StoreInfo } from "@/types/vendor"

// Form state keeps inputs as strings (money in pounds, postcodes as one
// comma-separated field) and converts to the API shape on save.
interface Draft {
  phone: string
  email: string
  line1: string
  line2: string
  city: string
  postcode: string
  collection: boolean
  lastOrderMinutes: string
  delivery: boolean
  fee: string
  minOrder: string
  freeOver: string
  postcodes: string
  hours: WeekHours
}

function toDraft(info: StoreInfo): Draft {
  const d = info.fulfilment.delivery
  return {
    phone: info.phone,
    email: info.email ?? "",
    line1: info.address.line1 ?? "",
    line2: info.address.line2 ?? "",
    city: info.address.city ?? "",
    postcode: info.address.postcode ?? "",
    collection: info.fulfilment.collection.enabled,
    lastOrderMinutes: String(info.fulfilment.collection.last_order_minutes),
    delivery: d.enabled,
    fee: penceToPoundsInput(d.fee),
    minOrder: penceToPoundsInput(d.min_order),
    freeOver: penceToPoundsInput(d.free_over),
    postcodes: d.postcode_prefixes.join(", "),
    hours: toWeekHours(info.opening_hours),
  }
}

const orNull = (s: string) => (s.trim() === "" ? null : s.trim())

// Returns the API payload, or field errors for money inputs that don't parse.
function fromDraft(
  d: Draft
): { info: StoreInfo } | { errors: Record<string, string> } {
  const fee = poundsInputToPence(d.fee) ?? 0
  const minOrder = poundsInputToPence(d.minOrder) ?? 0
  const freeOver = poundsInputToPence(d.freeOver)

  const errors: Record<string, string> = {}
  const money = "Enter an amount like 3.50"
  if (Number.isNaN(fee)) errors["fulfilment.delivery.fee"] = money
  if (Number.isNaN(minOrder)) errors["fulfilment.delivery.min_order"] = money
  if (freeOver !== null && Number.isNaN(freeOver))
    errors["fulfilment.delivery.free_over"] = money
  const lastOrderMinutes = Number(d.lastOrderMinutes.trim() || "0")
  if (
    !Number.isInteger(lastOrderMinutes) ||
    lastOrderMinutes < 0 ||
    lastOrderMinutes > 480
  ) {
    errors["fulfilment.collection.last_order_minutes"] =
      "Enter minutes between 0 and 480"
  }
  if (Object.keys(errors).length > 0) return { errors }

  return {
    info: {
      phone: d.phone.trim(),
      email: orNull(d.email),
      address: {
        line1: orNull(d.line1),
        line2: orNull(d.line2),
        city: orNull(d.city),
        postcode: orNull(d.postcode),
      },
      fulfilment: {
        collection: {
          enabled: d.collection,
          last_order_minutes: lastOrderMinutes,
        },
        delivery: {
          enabled: d.delivery,
          fee,
          min_order: minOrder,
          free_over: freeOver,
          postcode_prefixes: d.postcodes
            .split(/[,\n]/)
            .map((p) => p.trim())
            .filter(Boolean),
        },
      },
      opening_hours: fromWeekHours(d.hours),
    },
  }
}

// Store details editor, shared by the vendor portal and the admin side;
// each passes its own save server action.
function StoreInfoForm({
  branchId,
  initial,
  readOnly = false,
  saveAction,
  onSaved,
  submitLabel = "Save changes",
  requireChanges = true,
}: {
  branchId: string
  initial: StoreInfo
  readOnly?: boolean
  saveAction: (
    branchId: string,
    info: StoreInfo
  ) => Promise<ActionResult<StoreInfo>>
  onSaved?: () => void
  submitLabel?: string
  // false = submittable untouched (a setup step saving defaults)
  requireChanges?: boolean
}) {
  const [draft, setDraft] = useState(() => toDraft(initial))
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [dirty, setDirty] = useState(false)
  const [pending, startTransition] = useTransition()

  function set<K extends keyof Draft>(key: K, value: Draft[K]) {
    setDraft((d) => ({ ...d, [key]: value }))
    setDirty(true)
  }

  function save(e: React.FormEvent) {
    e.preventDefault()
    const result = fromDraft(draft)
    if ("errors" in result) {
      setErrors(result.errors)
      return
    }
    startTransition(async () => {
      const res = await saveAction(branchId, result.info)
      if (res.ok) {
        setDraft(toDraft(res.data)) // normalised by the API (e.g. postcodes)
        setErrors({})
        setDirty(false)
        toast.success("Store details saved")
        onSaved?.()
        return
      }
      setErrors(Object.fromEntries(res.issues.map((i) => [i.field, i.message])))
      toast.error(
        res.issues.length ? "Please fix the highlighted fields" : res.error
      )
    })
  }

  const text = (
    key: "phone" | "email" | "line1" | "line2" | "city" | "postcode",
    label: string,
    apiField: string,
    props: React.ComponentProps<typeof Input> = {}
  ) => (
    <Field data-invalid={!!errors[apiField] || undefined}>
      <FieldLabel htmlFor={key}>{label}</FieldLabel>
      <Input
        id={key}
        value={draft[key]}
        onChange={(e) => set(key, e.target.value)}
        aria-invalid={!!errors[apiField] || undefined}
        {...props}
      />
      {errors[apiField] && <FieldError>{errors[apiField]}</FieldError>}
    </Field>
  )

  const money = (
    key: "fee" | "minOrder" | "freeOver",
    label: string,
    apiField: string,
    description?: string
  ) => (
    <Field data-invalid={!!errors[apiField] || undefined}>
      <FieldLabel htmlFor={key}>{label}</FieldLabel>
      <div className="relative">
        <span className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-sm text-muted-foreground">
          £
        </span>
        <Input
          id={key}
          inputMode="decimal"
          className="pl-6"
          value={draft[key]}
          onChange={(e) => set(key, e.target.value)}
          aria-invalid={!!errors[apiField] || undefined}
        />
      </div>
      {description && <FieldDescription>{description}</FieldDescription>}
      {errors[apiField] && <FieldError>{errors[apiField]}</FieldError>}
    </Field>
  )

  return (
    <form onSubmit={save} className="flex flex-col gap-4">
      <fieldset disabled={readOnly || pending} className="contents">
        <Card>
          <CardHeader>
            <CardTitle>Contact and address</CardTitle>
            <CardDescription>Shown to customers at checkout.</CardDescription>
          </CardHeader>
          <CardContent>
            <FieldGroup className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {text("phone", "Phone", "phone", { type: "tel", required: true })}
              {text("email", "Email", "email", { type: "email" })}
              {text("line1", "Address line 1", "address.line1")}
              {text("line2", "Address line 2", "address.line2")}
              {text("city", "Town or city", "address.city")}
              {text("postcode", "Postcode", "address.postcode")}
            </FieldGroup>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Collection and delivery</CardTitle>
            <CardDescription>
              How customers can get their order.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <FieldGroup>
              <Field orientation="horizontal">
                <Switch
                  id="collection"
                  checked={draft.collection}
                  onCheckedChange={(v) => set("collection", v)}
                />
                <FieldContent>
                  <FieldLabel htmlFor="collection">
                    Collection in store
                  </FieldLabel>
                  <FieldDescription>
                    Same day only. Customers collect within an hour of being
                    told it&apos;s ready.
                  </FieldDescription>
                </FieldContent>
              </Field>

              {draft.collection && (
                <Field
                  className="animate-in duration-200 fade-in-0 sm:max-w-xs"
                  data-invalid={
                    !!errors["fulfilment.collection.last_order_minutes"] ||
                    undefined
                  }
                >
                  <FieldLabel htmlFor="last-order-minutes">
                    Last orders
                  </FieldLabel>
                  <div className="relative">
                    <Input
                      id="last-order-minutes"
                      inputMode="numeric"
                      className="pr-36"
                      value={draft.lastOrderMinutes}
                      onChange={(e) => set("lastOrderMinutes", e.target.value)}
                      aria-invalid={
                        !!errors["fulfilment.collection.last_order_minutes"] ||
                        undefined
                      }
                    />
                    <span className="pointer-events-none absolute top-1/2 right-2.5 -translate-y-1/2 text-sm text-muted-foreground">
                      minutes before closing
                    </span>
                  </div>
                  {errors["fulfilment.collection.last_order_minutes"] ? (
                    <FieldError>
                      {errors["fulfilment.collection.last_order_minutes"]}
                    </FieldError>
                  ) : (
                    <FieldDescription>
                      Leave time to pick the order and for the customer to
                      collect it.
                    </FieldDescription>
                  )}
                </Field>
              )}
              <Field orientation="horizontal">
                <Switch
                  id="delivery"
                  checked={draft.delivery}
                  onCheckedChange={(v) => set("delivery", v)}
                />
                <FieldContent>
                  <FieldLabel htmlFor="delivery">Local delivery</FieldLabel>
                </FieldContent>
              </Field>

              {draft.delivery && (
                <div className="grid animate-in grid-cols-1 gap-4 duration-200 fade-in-0 sm:grid-cols-3">
                  {money("fee", "Delivery fee", "fulfilment.delivery.fee")}
                  {money(
                    "minOrder",
                    "Minimum order",
                    "fulfilment.delivery.min_order"
                  )}
                  {money(
                    "freeOver",
                    "Free delivery over",
                    "fulfilment.delivery.free_over",
                    "Leave empty for no free delivery."
                  )}
                  <Field
                    className="sm:col-span-3"
                    data-invalid={
                      !!errors["fulfilment.delivery.postcode_prefixes"] ||
                      undefined
                    }
                  >
                    <FieldLabel htmlFor="postcodes">Delivery areas</FieldLabel>
                    <Input
                      id="postcodes"
                      value={draft.postcodes}
                      placeholder="E15, E20, E13"
                      onChange={(e) => set("postcodes", e.target.value)}
                      aria-invalid={
                        !!errors["fulfilment.delivery.postcode_prefixes"] ||
                        undefined
                      }
                    />
                    <FieldDescription>
                      The first part of the postcode, separated by commas.
                      &quot;LS1&quot; doesn&apos;t include LS10.
                    </FieldDescription>
                    {errors["fulfilment.delivery.postcode_prefixes"] && (
                      <FieldError>
                        {errors["fulfilment.delivery.postcode_prefixes"]}
                      </FieldError>
                    )}
                  </Field>
                </div>
              )}
            </FieldGroup>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Opening hours</CardTitle>
            <CardDescription>
              Your usual week. Add a second set of hours for a break, e.g.
              Friday prayers.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <OpeningHoursEditor
              value={draft.hours}
              onChange={(hours) => set("hours", hours)}
              errors={periodErrors(draft.hours, errors)}
              disabled={readOnly}
            />
          </CardContent>
        </Card>
      </fieldset>

      {!readOnly && (
        <div className="sticky bottom-0 -mx-4 flex items-center justify-end gap-3 border-t bg-background/80 px-4 py-3 backdrop-blur supports-backdrop-filter:bg-background/60 sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
          {dirty && (
            <span className="text-sm text-muted-foreground">
              Unsaved changes
            </span>
          )}
          <Button
            type="submit"
            size="lg"
            disabled={pending || (requireChanges && !dirty)}
          >
            {pending && <Loader2 className="animate-spin" />}
            {submitLabel}
          </Button>
        </div>
      )}
    </form>
  )
}

export { StoreInfoForm }
