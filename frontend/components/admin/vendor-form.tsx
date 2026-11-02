"use client"

import { useState, useTransition } from "react"
import { useRouter } from "next/navigation"
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
import { createVendorAction } from "@/lib/admin/actions"

// "Grocery Shop (Leeds) Ltd" -> "grocery-shop-leeds-ltd"
function slugify(name: string) {
  return name
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
}

// Step 1 of onboarding: the vendor (the business). Creating it moves on to
// the rest of the setup.
function VendorForm() {
  const router = useRouter()
  const [name, setName] = useState("")
  const [slug, setSlug] = useState("")
  const [slugEdited, setSlugEdited] = useState(false)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [pending, startTransition] = useTransition()

  function submit(e: React.FormEvent) {
    e.preventDefault()
    startTransition(async () => {
      const res = await createVendorAction({ name, slug })
      if (res.ok) {
        router.push(`/admin/vendors/${res.data.id}/setup`)
        return
      }
      setErrors(
        res.issues.length
          ? Object.fromEntries(res.issues.map((i) => [i.field, i.message]))
          : { name: res.error }
      )
    })
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-6">
      <FieldGroup>
        <Field data-invalid={!!errors.name || undefined}>
          <FieldLabel htmlFor="vendor-name">Business name</FieldLabel>
          <Input
            id="vendor-name"
            value={name}
            required
            autoFocus
            placeholder="e.g. Grocery Shop Leeds Ltd"
            aria-invalid={!!errors.name || undefined}
            onChange={(e) => {
              setName(e.target.value)
              if (!slugEdited) setSlug(slugify(e.target.value))
            }}
          />
          <FieldDescription>
            The owner&apos;s business. It can run one or more branches.
          </FieldDescription>
          {errors.name && <FieldError>{errors.name}</FieldError>}
        </Field>
        <Field data-invalid={!!errors.slug || undefined}>
          <FieldLabel htmlFor="vendor-slug">Short name</FieldLabel>
          <Input
            id="vendor-slug"
            value={slug}
            required
            className="font-mono"
            aria-invalid={!!errors.slug || undefined}
            onChange={(e) => {
              setSlug(e.target.value)
              setSlugEdited(true)
            }}
          />
          <FieldDescription>
            Lowercase letters, numbers and hyphens. Used in links and reports.
          </FieldDescription>
          {errors.slug && <FieldError>{errors.slug}</FieldError>}
        </Field>
      </FieldGroup>
      <Button type="submit" size="lg" className="w-fit" disabled={pending}>
        {pending && <Loader2 className="animate-spin" />}
        Create and continue
      </Button>
    </form>
  )
}

export { VendorForm }
