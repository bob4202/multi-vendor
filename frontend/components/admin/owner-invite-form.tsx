"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
import { Loader2 } from "lucide-react"

import { InviteLinkPanel } from "@/components/admin/invite-link-panel"
import { Button } from "@/components/ui/button"
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { inviteUserAction, type InviteLink } from "@/lib/admin/actions"

// Onboarding step: invite the vendor's owner. Once created, the link stays
// on screen until the admin moves on - it's the only time it's visible.
function OwnerInviteForm({
  vendorId,
  vendorName,
  nextHref,
}: {
  vendorId: string
  vendorName: string
  nextHref: string
}) {
  const router = useRouter()
  const [name, setName] = useState("")
  const [email, setEmail] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [invite, setInvite] = useState<InviteLink | null>(null)
  const [pending, startTransition] = useTransition()

  function submit(e: React.FormEvent) {
    e.preventDefault()
    startTransition(async () => {
      const res = await inviteUserAction(vendorId, {
        name: name.trim(),
        email: email.trim(),
        role: "owner",
      })
      if (res.ok) {
        setInvite(res.data)
        setErrors({})
        return
      }
      setErrors(
        res.issues.length
          ? Object.fromEntries(res.issues.map((i) => [i.field, i.message]))
          : { email: res.error }
      )
    })
  }

  if (invite) {
    return (
      <div className="flex flex-col gap-6">
        <InviteLinkPanel invite={invite} name={name} vendorName={vendorName} />
        <Button
          size="lg"
          className="w-fit"
          onClick={() => {
            router.push(nextHref)
            router.refresh()
          }}
        >
          I&apos;ve sent it, continue
        </Button>
      </div>
    )
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-6">
      <fieldset disabled={pending} className="contents">
        <FieldGroup className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field data-invalid={!!errors.name || undefined}>
            <FieldLabel htmlFor="owner-name">Owner&apos;s name</FieldLabel>
            <Input
              id="owner-name"
              value={name}
              required
              onChange={(e) => setName(e.target.value)}
              aria-invalid={!!errors.name || undefined}
            />
            {errors.name && <FieldError>{errors.name}</FieldError>}
          </Field>
          <Field data-invalid={!!errors.email || undefined}>
            <FieldLabel htmlFor="owner-email">Owner&apos;s email</FieldLabel>
            <Input
              id="owner-email"
              type="email"
              value={email}
              required
              onChange={(e) => setEmail(e.target.value)}
              aria-invalid={!!errors.email || undefined}
            />
            {errors.email && <FieldError>{errors.email}</FieldError>}
          </Field>
        </FieldGroup>
        <p className="text-sm text-muted-foreground">
          You&apos;ll get a link to send them. They choose their own password,
          so nobody else ever knows it.
        </p>
        <Button type="submit" size="lg" className="w-fit">
          {pending && <Loader2 className="animate-spin" />}
          Create invite link
        </Button>
      </fieldset>
    </form>
  )
}

export { OwnerInviteForm }
