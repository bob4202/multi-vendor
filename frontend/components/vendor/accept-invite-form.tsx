"use client"

import { useActionState } from "react"
import { Loader2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import type { AcceptInviteState } from "@/lib/vendor/actions"

// Choose a password from an invite link. `action` is acceptInviteAction
// already bound to the link's token.
function AcceptInviteForm({
  email,
  action,
}: {
  email: string
  action: (
    prev: AcceptInviteState,
    formData: FormData
  ) => Promise<AcceptInviteState>
}) {
  const [state, formAction, pending] = useActionState(action, {})

  return (
    <form action={formAction} className="flex flex-col gap-6">
      {/* Lets password managers save the login against the right email. */}
      <input
        type="email"
        name="username"
        value={email}
        autoComplete="username"
        readOnly
        hidden
      />
      <FieldGroup>
        <Field data-invalid={!!state.error || undefined}>
          <FieldLabel htmlFor="password">New password</FieldLabel>
          <Input
            id="password"
            name="password"
            type="password"
            autoComplete="new-password"
            minLength={12}
            required
            autoFocus
            aria-invalid={!!state.error || undefined}
          />
          <FieldDescription>At least 12 characters.</FieldDescription>
        </Field>
        <Field data-invalid={!!state.error || undefined}>
          <FieldLabel htmlFor="confirm">Confirm password</FieldLabel>
          <Input
            id="confirm"
            name="confirm"
            type="password"
            autoComplete="new-password"
            minLength={12}
            required
            aria-invalid={!!state.error || undefined}
          />
          {state.error && <FieldError>{state.error}</FieldError>}
        </Field>
      </FieldGroup>
      <Button type="submit" size="lg" disabled={pending}>
        {pending && <Loader2 className="animate-spin" />}
        Set password and sign in
      </Button>
    </form>
  )
}

export { AcceptInviteForm }
