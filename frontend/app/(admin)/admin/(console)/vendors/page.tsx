import type { Metadata } from "next"
import Link from "next/link"
import { ChevronRight, Plus, Search, Store } from "lucide-react"

import { EmptyState } from "@/components/common/empty-state"
import { PageHeader } from "@/components/common/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { adminAPI } from "@/lib/admin/api"

export const metadata: Metadata = { title: "Vendors" }

type Props = { searchParams: Promise<{ q?: string }> }

function plural(n: number, word: string) {
  return `${n} ${word}${n === 1 ? "" : "s"}`
}

export default async function VendorsPage({ searchParams }: Props) {
  const { q } = await searchParams
  const vendors = await adminAPI.listVendors(q)

  return (
    <>
      <PageHeader
        title="Vendors"
        description="Businesses trading on the platform, and their branches."
        actions={
          <Button
            nativeButton={false}
            render={<Link href="/admin/vendors/new" />}
          >
            <Plus />
            Onboard a vendor
          </Button>
        }
      />

      <form action="/admin/vendors" className="mb-4 flex gap-2">
        <div className="relative flex-1">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            name="q"
            type="search"
            defaultValue={q}
            placeholder="Search by name or slug"
            className="pl-8"
          />
        </div>
        <Button type="submit" variant="outline">
          Search
        </Button>
      </form>

      {vendors.length === 0 ? (
        <EmptyState
          icon={Store}
          title={q ? "No vendors match" : "No vendors yet"}
          description={
            q
              ? `Nothing matches "${q}".`
              : "Onboard your first vendor to get started."
          }
          className="rounded-xl border"
        />
      ) : (
        <ul className="flex flex-col divide-y rounded-xl border">
          {vendors.map((v) => (
            <li key={v.id}>
              <Link
                href={`/admin/vendors/${v.id}`}
                className="flex items-center gap-3 px-4 py-3 transition-colors hover:bg-muted/50"
              >
                <div className="flex min-w-0 flex-1 flex-col">
                  <span className="flex items-center gap-2 font-medium">
                    <span className="truncate">{v.name}</span>
                    {!v.is_active && (
                      <Badge variant="outline">Deactivated</Badge>
                    )}
                  </span>
                  <span className="truncate text-xs text-muted-foreground">
                    {v.slug} · {plural(v.branch_count, "branch")} ·{" "}
                    {plural(v.user_count, "user")}
                  </span>
                </div>
                {v.pending_invites > 0 && (
                  <Badge className="bg-amber-500/15 text-amber-700 dark:text-amber-400">
                    {plural(v.pending_invites, "pending invite")}
                  </Badge>
                )}
                <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
              </Link>
            </li>
          ))}
        </ul>
      )}
    </>
  )
}
