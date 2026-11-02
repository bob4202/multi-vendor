import type { Metadata } from "next"
import Link from "next/link"
import { redirect } from "next/navigation"
import { ChevronRight, Store } from "lucide-react"

import { EmptyState } from "@/components/common/empty-state"
import { Badge } from "@/components/ui/badge"
import { UserMenu } from "@/components/portal/user-menu"
import { logoutAction } from "@/lib/vendor/actions"
import { vendorPortalUser } from "@/lib/vendor/portal-user"
import { requireSession } from "@/lib/vendor/session"

export const metadata: Metadata = {
  title: { absolute: "Choose a branch | Vendor portal" },
}

// Entry point after login: straight into the only branch, or a picker for
// owners running several.
export default async function VendorHomePage() {
  const session = await requireSession()
  const { branches } = session

  if (branches.length === 1) {
    redirect(`/vendor/${branches[0].id}/orders`)
  }

  return (
    <div className="min-h-svh bg-muted/30 px-4 py-12">
      <div className="mx-auto flex w-full max-w-lg flex-col gap-6">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">
              {session.vendor.name}
            </h1>
            <p className="text-sm text-muted-foreground">
              Choose a branch to manage.
            </p>
          </div>
          <UserMenu
            user={vendorPortalUser(session)}
            logoutAction={logoutAction}
          />
        </div>

        {branches.length === 0 ? (
          <EmptyState
            icon={Store}
            title="No branches yet"
            description="You haven't been given access to any branch. Ask your owner, or contact Grocery Shop support."
            className="rounded-xl border bg-background"
          />
        ) : (
          <ul className="flex flex-col gap-2">
            {branches.map((b) => (
              <li key={b.id}>
                <Link
                  href={`/vendor/${b.id}/orders`}
                  className="flex items-center gap-3 rounded-xl border bg-background p-4 transition-colors hover:bg-muted/50"
                >
                  <span className="flex size-10 items-center justify-center rounded-lg bg-muted">
                    <Store className="size-5 text-muted-foreground" />
                  </span>
                  <span className="flex-1 font-medium">{b.name}</span>
                  {!b.is_active && <Badge variant="outline">Inactive</Badge>}
                  <ChevronRight className="size-4 text-muted-foreground" />
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
