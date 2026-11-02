"use client"

import { useState, useTransition } from "react"
import { Loader2, TriangleAlert } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { saveBranchAction } from "@/lib/admin/actions"
import type { AdminBranch } from "@/types/admin"
import type { Category } from "@/types/category"

// A branch's inventory system connection: where its products sync from, and
// which product departments to put online. Create when `branch` is absent,
// edit otherwise. The API key is write-only: it's never shown again, and
// leaving it blank on edit keeps the current one.
function BranchForm({
  vendorId,
  branch,
  categories,
  onSaved,
  submitLabel = "Save branch",
}: {
  vendorId: string
  branch?: AdminBranch
  categories: Category[]
  onSaved?: (branch: AdminBranch) => void
  submitLabel?: string
}) {
  const [name, setName] = useState(branch?.name ?? "")
  const [host, setHost] = useState(branch?.host ?? "")
  const [username, setUsername] = useState(branch?.api_username ?? "")
  const [apiKey, setApiKey] = useState("")
  const [storefrontUrl, setStorefrontUrl] = useState(
    branch?.storefront_url ?? ""
  )
  const [active, setActive] = useState(branch?.is_active ?? true)
  const [departments, setDepartments] = useState<string[]>(
    branch?.allowed_secondary_department_uuids ?? []
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [pending, startTransition] = useTransition()

  function toggleDepartment(id: string, on: boolean) {
    setDepartments((d) => (on ? [...d, id] : d.filter((x) => x !== id)))
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!branch && !apiKey.trim()) {
      setErrors({ api_key: "Enter the branch's inventory system API key" })
      return
    }
    startTransition(async () => {
      const res = await saveBranchAction(vendorId, branch?.id ?? null, {
        name: name.trim(),
        host: host.trim(),
        api_username: username.trim(),
        api_key: apiKey.trim(),
        is_active: active,
        allowed_secondary_department_uuids: departments,
        storefront_url: storefrontUrl.trim(),
      })
      if (res.ok) {
        setApiKey("")
        setErrors({})
        toast.success(branch ? "Branch saved" : "Branch created")
        onSaved?.(res.data)
        return
      }
      setErrors(
        res.issues.length
          ? Object.fromEntries(res.issues.map((i) => [i.field, i.message]))
          : { name: res.error }
      )
    })
  }

  const field = (
    id: string,
    label: string,
    value: string,
    set: (v: string) => void,
    props: React.ComponentProps<typeof Input> = {},
    description?: string
  ) => (
    <Field data-invalid={!!errors[id] || undefined}>
      <FieldLabel htmlFor={`branch-${id}`}>{label}</FieldLabel>
      <Input
        id={`branch-${id}`}
        value={value}
        onChange={(e) => set(e.target.value)}
        aria-invalid={!!errors[id] || undefined}
        {...props}
      />
      {description && <FieldDescription>{description}</FieldDescription>}
      {errors[id] && <FieldError>{errors[id]}</FieldError>}
    </Field>
  )

  return (
    <form onSubmit={submit} className="flex flex-col gap-6">
      <fieldset disabled={pending} className="contents">
        <FieldGroup className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {field("name", "Branch name", name, setName, {
            required: true,
            placeholder: "e.g. Grocery Leeds",
          })}
          {field(
            "host",
            "Inventory system host",
            host,
            setHost,
            {
              required: true,
              placeholder: "shop-leeds",
              className: "font-mono",
            },
            "The branch's host name in the inventory system."
          )}
          {field(
            "api_username",
            "API username",
            username,
            setUsername,
            {
              required: true,
              autoComplete: "off",
            }
          )}
          {field(
            "api_key",
            "API key",
            apiKey,
            setApiKey,
            {
              type: "password",
              autoComplete: "new-password",
              placeholder: branch
                ? "Leave blank to keep the current key"
                : undefined,
            },
            "Stored encrypted. It's never shown again."
          )}
          <div className="sm:col-span-2">
            {field(
              "storefront_url",
              "Storefront URL",
              storefrontUrl,
              setStorefrontUrl,
              {
                type: "url",
                inputMode: "url",
                placeholder: "https://leeds.groceryshop.example",
              },
              "Where this branch's shop lives. Order emails link here; leave empty for emails without links."
            )}
          </div>
        </FieldGroup>

        <FieldSet>
          <FieldLegend variant="label">
            Product departments to sell online
          </FieldLegend>
          <FieldDescription>
            Only products in these departments are synced to the shop.
          </FieldDescription>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            {categories.map((c) => (
              <Field key={c.id} orientation="horizontal">
                <Checkbox
                  id={`dept-${c.id}`}
                  checked={departments.includes(c.id)}
                  onCheckedChange={(on) => toggleDepartment(c.id, on === true)}
                />
                <FieldLabel htmlFor={`dept-${c.id}`} className="font-normal">
                  {c.name}
                </FieldLabel>
              </Field>
            ))}
          </div>
          {departments.length === 0 && (
            <p className="flex items-center gap-2 rounded-lg bg-amber-500/10 px-3 py-2 text-sm text-amber-800 dark:text-amber-300">
              <TriangleAlert className="size-4 shrink-0" />
              With no departments chosen, nothing will sync and the shop will be
              empty.
            </p>
          )}
        </FieldSet>

        {branch && (
          <Field orientation="horizontal">
            <Switch
              id="branch-active"
              checked={active}
              onCheckedChange={setActive}
            />
            <FieldContent>
              <FieldLabel htmlFor="branch-active">Active</FieldLabel>
              <FieldDescription>
                Inactive branches stop syncing and their storefront goes
                offline.
              </FieldDescription>
            </FieldContent>
          </Field>
        )}

        <Button type="submit" size="lg" className="w-fit">
          {pending && <Loader2 className="animate-spin" />}
          {submitLabel}
        </Button>
      </fieldset>
    </form>
  )
}

export { BranchForm }
