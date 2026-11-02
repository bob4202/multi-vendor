import type { Metadata } from "next"
import { CircleAlert, CircleCheck, PackageSearch, Search } from "lucide-react"

import { EmptyState } from "@/components/common/empty-state"
import { Price } from "@/components/common/price"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { LinkTabs } from "@/components/common/link-tabs"
import { PageHeader } from "@/components/common/page-header"
import { SimplePagination } from "@/components/common/simple-pagination"
import { SyncButton } from "@/components/portal/sync-button"
import { triggerSyncAction } from "@/lib/vendor/actions"
import { formatDateTime } from "@/lib/format"
import { vendorAPI } from "@/lib/vendor/api"
import { requireBranch } from "@/lib/vendor/session"
import type { Product } from "@/types/product"
import type { SyncState } from "@/types/vendor"

export const metadata: Metadata = { title: "Products" }

type Props = {
  params: Promise<{ branchId: string }>
  searchParams: Promise<{ q?: string; category?: string; page?: string }>
}

function availability(p: Product) {
  if (p.deleted) return { label: "Deleted", variant: "outline" as const }
  if (!p.can_be_sold)
    return { label: "Not for sale", variant: "outline" as const }
  return { label: "Online", variant: "secondary" as const }
}

function SyncStatus({ state }: { state: SyncState | undefined }) {
  if (!state?.last_run_at) {
    return <span className="text-muted-foreground">Never synced</span>
  }
  const failed = state.last_run_status === "error"
  const Icon = failed ? CircleAlert : CircleCheck
  return (
    <span className="flex flex-col gap-0.5">
      <span className="flex items-center gap-1.5">
        <Icon
          className={
            failed ? "size-4 text-destructive" : "size-4 text-emerald-600"
          }
        />
        {failed ? "Last sync failed" : "Last synced"}{" "}
        {formatDateTime(state.last_run_at)}
      </span>
      {failed && state.last_run_error && (
        <span className="text-xs text-muted-foreground">
          {state.last_run_error}
        </span>
      )}
    </span>
  )
}

export default async function ProductsPage({ params, searchParams }: Props) {
  const { branchId } = await params
  const { q, category, page: pageParam } = await searchParams
  const page = Math.max(1, Number(pageParam) || 1)

  const { session } = await requireBranch(branchId)
  const [products, categories, syncStates] = await Promise.all([
    vendorAPI.listProducts(branchId, { search: q, category, page }),
    vendorAPI.listCategories(branchId),
    vendorAPI.syncState(branchId),
  ])

  const base = `/vendor/${branchId}/products`
  const hrefFor = (next: { category?: string; page?: number }) => {
    const params = new URLSearchParams()
    if (q) params.set("q", q)
    if (next.category) params.set("category", next.category)
    if (next.page && next.page > 1) params.set("page", String(next.page))
    const qs = params.toString()
    return qs ? `${base}?${qs}` : base
  }

  return (
    <>
      <PageHeader
        title="Products"
        description="Prices and availability come from your inventory system."
        actions={
          session.permissions.trigger_sync && (
            <SyncButton branchId={branchId} syncAction={triggerSyncAction} />
          )
        }
      />

      <div className="mb-4 rounded-xl border p-4 text-sm">
        <SyncStatus state={syncStates.find((s) => s.resource === "product")} />
      </div>

      <div className="mb-4 flex flex-col gap-3">
        <form action={base} className="flex gap-2">
          {category && <input type="hidden" name="category" value={category} />}
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              name="q"
              type="search"
              defaultValue={q}
              placeholder="Search products"
              className="pl-8"
            />
          </div>
          <Button type="submit" variant="outline">
            Search
          </Button>
        </form>
        <LinkTabs
          tabs={[
            { href: hrefFor({}), label: "All", active: !category },
            ...categories.map((c) => ({
              href: hrefFor({ category: c.id }),
              label: c.name,
              active: c.id === category,
            })),
          ]}
        />
      </div>

      {products.data.length === 0 ? (
        <EmptyState
          icon={PackageSearch}
          title="No products found"
          description={
            q
              ? `Nothing matches "${q}".`
              : "Nothing has synced for this branch yet."
          }
          className="rounded-xl border"
        />
      ) : (
        <ul className="flex flex-col divide-y rounded-xl border">
          {products.data.map((p) => {
            const status = availability(p)
            return (
              <li key={p.id} className="flex items-center gap-3 px-4 py-3">
                <div className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate font-medium">
                    {p.name ?? "Unnamed product"}
                  </span>
                  <span className="truncate text-xs text-muted-foreground">
                    {[p.plu && `PLU ${p.plu}`, p.category_name]
                      .filter(Boolean)
                      .join(" · ")}
                  </span>
                </div>
                <Badge
                  variant={status.variant}
                  className="hidden sm:inline-flex"
                >
                  {status.label}
                </Badge>
                <div className="flex flex-col items-end">
                  <Price pence={p.price} className="font-medium" />
                  <span className="text-xs text-muted-foreground sm:hidden">
                    {status.label}
                  </span>
                </div>
              </li>
            )
          })}
        </ul>
      )}

      <SimplePagination
        page={products.page}
        totalPages={products.total_pages}
        hrefForPage={(p) => hrefFor({ category, page: p })}
      />
    </>
  )
}
