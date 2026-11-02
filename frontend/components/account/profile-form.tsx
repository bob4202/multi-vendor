"use client"

import { useActionState } from "react"
import { Check, Loader2 } from "lucide-react"

import { FormField } from "@/components/common/form-field"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  updateProfileAction,
  type ProfileState,
} from "@/lib/customer/actions"
import type { Customer } from "@/types/customer"

// Name and phone, used to fill in checkout. The email is the login, so it
// can't be changed here.
function ProfileForm({ customer }: { customer: Customer }) {
  const [state, formAction, pending] = useActionState<ProfileState, FormData>(
    updateProfileAction,
    {}
  )
  const current = state.customer ?? customer
  const fieldError = (f: string) => state.fieldErrors?.[f]

  return (
    <form action={formAction} className="flex flex-col gap-4">
      <FormField id="profile-email" label="Email">
        <Input id="profile-email" value={current.email} disabled readOnly />
      </FormField>
      <FormField id="profile-name" label="Full name" error={fieldError("name")}>
        <Input
          id="profile-name"
          name="name"
          autoComplete="name"
          defaultValue={current.name ?? ""}
          aria-invalid={!!fieldError("name")}
        />
      </FormField>
      <FormField
        id="profile-phone"
        label="Phone"
        error={fieldError("phone")}
        hint="So the shop can reach you about an order"
      >
        <Input
          id="profile-phone"
          name="phone"
          type="tel"
          autoComplete="tel"
          defaultValue={current.phone ?? ""}
          aria-invalid={!!fieldError("phone")}
        />
      </FormField>
      {state.error && <p className="text-sm text-destructive">{state.error}</p>}
      <div className="flex items-center gap-3">
        <Button type="submit" disabled={pending}>
          {pending && <Loader2 className="animate-spin" />}
          Save
        </Button>
        {state.ok && !pending && (
          <span className="flex animate-in items-center gap-1 text-sm text-emerald-600 fade-in-0 dark:text-emerald-400">
            <Check className="size-4" />
            Saved
          </span>
        )}
      </div>
    </form>
  )
}

export { ProfileForm }
