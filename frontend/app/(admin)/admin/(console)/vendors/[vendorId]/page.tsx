import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft, Plus, Store, Users } from "lucide-react"

import { EmptyState } from "@/components/common/empty-state"
import { PageHeader } from "@/components/common/page-header"
import { ResendInviteButton } from "@/components/admin/resend-invite-button"
import { SyncStatus } from "@/components/admin/sync-status"
import { VendorStatusButton } from "@/components/admin/vendor-status-button"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { adminAPI } from "@/lib/admin/api"
import { formatDateTime } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { AdminVendorUserStatus } from "@/types/admin"

export const metadata: Metadata = { title: "Vendor" }

const ROLE_LABEL = { owner: "Owner", manager: "Manager", staff: "Staff" }

const USER_STATUS: Record<
  AdminVendorUserStatus,
  { label: string; tone: string }
> = {
  active: {
    label: "Active",
    tone: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400",
  },
  invited: {
    label: "Invited",
    tone: "bg-sky-500/15 text-sky-700 dark:text-sky-400",
  },
  invite_expired: {
    label: "Invite expired",
    tone: "bg-amber-500/15 text-amber-700 dark:text-amber-400",
  },
  locked: { label: "Locked out", tone: "bg-destructive/10 text-destructive" },
  disabled: { label: "Disabled", tone: "bg-muted text-muted-foreground" },
}

// A green tick or a muted "missing" for one setup item.
function Check({ ok, label }: { ok: boolean; label: string }) {
  return (
    <Badge
      variant="outline"
      className={cn(
        !ok && "border-amber-500/40 text-amber-700 dark:text-amber-400"
      )}
    >
      {ok ? label : `No ${label.toLowerCase()}`}
    </Badge>
  )
}

export default async function AdminVendorPage({
  params,
}: {
  params: Promise<{ vendorId: string }>
}) {
  const { vendorId } = await params
  const vendor = await adminAPI.getVendor(vendorId)
  if (!vendor) notFound()

  const hasOwner = vendor.users.some((u) => u.role === "owner")
  const setupIncomplete =
    !hasOwner ||
    vendor.branches.length === 0 ||
    vendor.branches.some(
      (b) =>
        !b.has_store_details ||
        !b.storefront_api_key_prefix ||
        b.last_sync_status !== "success"
    )

  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        className="mb-4 -ml-2"
        nativeButton={false}
        render={<Link href="/admin/vendors" />}
      >
        <ArrowLeft />
        Vendors
      </Button>
      <PageHeader
        title={
          <span className="flex flex-wrap items-center gap-3">
            {vendor.name}
            {!vendor.is_active && <Badge variant="outline">Deactivated</Badge>}
          </span>
        }
        description={vendor.slug}
        actions={
          <>
            {setupIncomplete && vendor.is_active && (
              <Button
                nativeButton={false}
                render={<Link href={`/admin/vendors/${vendor.id}/setup`} />}
              >
                Continue setup
              </Button>
            )}
            <VendorStatusButton
              vendorId={vendor.id}
              vendorName={vendor.name}
              isActive={vendor.is_active}
            />
          </>
        }
      />

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Card className="min-w-0">
          <CardHeader>
            <CardTitle>Branches</CardTitle>
            <CardAction>
              <Button
                variant="outline"
                size="sm"
                nativeButton={false}
                render={
                  <Link href={`/admin/vendors/${vendor.id}/setup?branch=new`} />
                }
              >
                <Plus />
                Add branch
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent>
            {vendor.branches.length === 0 ? (
              <EmptyState
                icon={Store}
                title="No branches yet"
                className="py-6"
              />
            ) : (
              <ul className="flex flex-col divide-y">
                {vendor.branches.map((b) => (
                  <li
                    key={b.id}
                    className="flex flex-col gap-2 py-3 first:pt-0 last:pb-0"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <Link
                        href={`/admin/vendors/${vendor.id}/branches/${b.id}`}
                        className="min-w-0 truncate font-medium hover:underline"
                      >
                        {b.name}
                      </Link>
                      {!b.is_active && (
                        <Badge variant="outline">Inactive</Badge>
                      )}
                    </div>
                    <div className="flex flex-wrap gap-1.5">
                      <Check ok={b.has_store_details} label="Store details" />
                      <Check
                        ok={!!b.storefront_api_key_prefix}
                        label="Storefront key"
                      />
                    </div>
                    <SyncStatus
                      at={b.last_sync_at}
                      status={b.last_sync_status}
                      error={b.last_sync_error}
                    />
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <Card className="min-w-0">
          <CardHeader>
            <CardTitle>People</CardTitle>
            {!hasOwner && (
              <CardAction>
                <Button
                  variant="outline"
                  size="sm"
                  nativeButton={false}
                  render={
                    <Link href={`/admin/vendors/${vendor.id}/setup?step=3`} />
                  }
                >
                  Invite owner
                </Button>
              </CardAction>
            )}
          </CardHeader>
          <CardContent>
            {vendor.users.length === 0 ? (
              <EmptyState
                icon={Users}
                title="Nobody can sign in yet"
                className="py-6"
              />
            ) : (
              <ul className="flex flex-col divide-y">
                {vendor.users.map((u) => {
                  const status = USER_STATUS[u.status]
                  const canResend =
                    u.status === "invited" || u.status === "invite_expired"
                  return (
                    <li
                      key={u.id}
                      className="flex flex-col gap-2 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between"
                    >
                      <div className="flex min-w-0 flex-col">
                        <span className="flex items-center gap-2">
                          <span className="truncate font-medium">{u.name}</span>
                          <Badge className={status.tone}>{status.label}</Badge>
                        </span>
                        <span className="truncate text-xs text-muted-foreground">
                          {ROLE_LABEL[u.role]} · {u.email}
                          {u.last_login_at &&
                            ` · last signed in ${formatDateTime(u.last_login_at)}`}
                          {u.status === "invited" &&
                            u.invite_expires_at &&
                            ` · invite expires ${formatDateTime(u.invite_expires_at)}`}
                        </span>
                      </div>
                      {canResend && (
                        <ResendInviteButton
                          vendorId={vendor.id}
                          vendorName={vendor.name}
                          userId={u.id}
                          userName={u.name}
                        />
                      )}
                    </li>
                  )
                })}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  )
}
