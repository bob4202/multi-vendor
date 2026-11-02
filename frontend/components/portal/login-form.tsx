"use client"

import { useActionState } from "react"
import { Loader2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import type { LoginState } from "@/lib/portal-session"

// Email + password form for either portal; `action` is that portal's login
// server action.
function LoginForm({
  action,
  next,
  notice,
}: {
  action: (prev: LoginState, formData: FormData) => Promise<LoginState>
  next?: string
  notice?: string
}) {
  const [state, formAction, pending] = useActionState(action, {})

  return (
    <form action={formAction} className="flex flex-col gap-6">
      <input type="hidden" name="next" value={next ?? ""} />
      {notice && !state.error && (
        <p className="rounded-lg bg-muted px-3 py-2 text-sm text-muted-foreground">
          {notice}
        </p>
      )}
      <FieldGroup>
        <Field data-invalid={!!state.error || undefined}>
          <FieldLabel htmlFor="email">Email</FieldLabel>
          <Input
            id="email"
            name="email"
            type="email"
            autoComplete="username"
            required
            autoFocus
            defaultValue={state.email}
            aria-invalid={!!state.error || undefined}
          />
        </Field>
        <Field data-invalid={!!state.error || undefined}>
          <FieldLabel htmlFor="password">Password</FieldLabel>
          <Input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            required
            aria-invalid={!!state.error || undefined}
          />
          {state.error && <FieldError>{state.error}</FieldError>}
        </Field>
      </FieldGroup>
      <Button type="submit" size="lg" disabled={pending}>
        {pending && <Loader2 className="animate-spin" />}
        Sign in
      </Button>
    </form>
  )
}

export { LoginForm }
