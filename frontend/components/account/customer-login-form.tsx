"use client"

import { useActionState, useEffect, useRef, useState } from "react"
import { Loader2, MailCheck } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
} from "@/components/ui/input-otp"
import {
  customerLoginAction,
  type LoginState,
} from "@/lib/customer/actions"

const RESEND_SECONDS = 60

// Sign in without a password: email first, then the 6-digit code we email.
// One server action handles both steps; `intent` says which.
function CustomerLoginForm({ next }: { next?: string }) {
  const [state, formAction, pending] = useActionState<LoginState, FormData>(
    customerLoginAction,
    { step: "email" }
  )
  const [code, setCode] = useState("")
  const formRef = useRef<HTMLFormElement>(null)

  // A new code step (first send or resend) starts with an empty box.
  useEffect(() => {
    if (state.step === "code" && !state.error) setCode("")
  }, [state])

  if (state.step === "email") {
    return (
      <form action={formAction} className="flex flex-col gap-6">
        <input type="hidden" name="intent" value="send" />
        <FieldGroup>
          <Field data-invalid={!!state.error || undefined}>
            <FieldLabel htmlFor="email">Email</FieldLabel>
            <Input
              id="email"
              name="email"
              type="email"
              autoComplete="email"
              required
              autoFocus
              defaultValue={state.email}
              aria-invalid={!!state.error || undefined}
            />
            {state.error ? (
              <FieldError>{state.error}</FieldError>
            ) : (
              <FieldDescription>
                We&apos;ll email you a code. No password needed.
              </FieldDescription>
            )}
          </Field>
        </FieldGroup>
        <Button type="submit" size="lg" disabled={pending}>
          {pending && <Loader2 className="animate-spin" />}
          Email me a code
        </Button>
      </form>
    )
  }

  return (
    <form
      ref={formRef}
      action={formAction}
      className="flex animate-in flex-col gap-6 duration-200 fade-in-0"
    >
      <input type="hidden" name="email" value={state.email ?? ""} />
      <input type="hidden" name="next" value={next ?? ""} />
      <input type="hidden" name="code" value={code} />

      <div className="flex items-start gap-3 rounded-lg bg-muted px-3 py-2.5 text-sm">
        <MailCheck className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        <p>
          We sent a code to <span className="font-medium">{state.email}</span>.
          It works for 10 minutes.
        </p>
      </div>

      <Field data-invalid={!!state.error || undefined}>
        <FieldLabel htmlFor="code">Code</FieldLabel>
        <InputOTP
          id="code"
          maxLength={6}
          inputMode="numeric"
          pattern="^[0-9]*$"
          autoComplete="one-time-code"
          autoFocus
          value={code}
          onChange={setCode}
          // Pasting or typing the last digit signs in straight away.
          onComplete={() => formRef.current?.requestSubmit()}
          aria-invalid={!!state.error || undefined}
          containerClassName="justify-center"
        >
          <InputOTPGroup>
            {Array.from({ length: 6 }, (_, i) => (
              <InputOTPSlot
                key={i}
                index={i}
                className="size-11 text-lg"
                aria-invalid={!!state.error || undefined}
              />
            ))}
          </InputOTPGroup>
        </InputOTP>
        {state.error && <FieldError>{state.error}</FieldError>}
      </Field>

      <Button
        type="submit"
        name="intent"
        value="verify"
        size="lg"
        disabled={pending || code.length !== 6}
      >
        {pending && <Loader2 className="animate-spin" />}
        Sign in
      </Button>

      <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
        <ResendButton disabled={pending} />
        <Button
          type="button"
          variant="link"
          className="h-auto p-0"
          onClick={() => window.location.reload()}
        >
          Use a different email
        </Button>
      </div>
    </form>
  )
}

// "Send a new code", available once a minute has passed.
function ResendButton({ disabled }: { disabled: boolean }) {
  const [left, setLeft] = useState(RESEND_SECONDS)
  useEffect(() => {
    if (left <= 0) return
    const t = setTimeout(() => setLeft((s) => s - 1), 1000)
    return () => clearTimeout(t)
  }, [left])

  return (
    <Button
      type="submit"
      name="intent"
      value="resend"
      variant="link"
      className="h-auto p-0"
      disabled={disabled || left > 0}
      onClick={() => setLeft(RESEND_SECONDS)}
    >
      {left > 0 ? `Send a new code in ${left}s` : "Send a new code"}
    </Button>
  )
}

export { CustomerLoginForm }
