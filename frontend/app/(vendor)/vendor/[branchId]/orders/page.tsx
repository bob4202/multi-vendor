import type { Metadata } from "next"
import { Inbox } from "lucide-react"

import { EmptyState } from "@/components/common/empty-state"
import { FindByCode } from "@/components/vendor/find-by-code"
import { LiveOrders } from "@/components/vendor/live-orders"
import { LinkTabs } from "@/components/common/link-tabs"
import { OrderList } from "@/components/vendor/order-list"
import { PageHeader } from "@/components/common/page-header"
import { SimplePagination } from "@/components/common/simple-pagination"
import { vendorAPI, type OrderListFilter } from "@/lib/vendor/api"
import { requireBranch } from "@/lib/vendor/session"

export const metadata: Metadata = { title: "Orders" }

const FILTERS: { value: OrderListFilter; label: string; empty: string }[] = [
  {
    value: "active",
    label: "To do",
    empty: "No orders waiting. New orders show up here automatically.",
  },
  { value: "done", label: "Done", empty: "No finished orders yet." },
  { value: "all", label: "All", empty: "No orders yet." },
]

type Props = {
  params: Promise<{ branchId: string }>
  searchParams: Promise<{ status?: string; page?: string }>
}

export default async function OrdersPage({ params, searchParams }: Props) {
  const { branchId } = await params
  const query = await searchParams
  const filter = FILTERS.find((f) => f.value === query.status) ?? FILTERS[0]
  const page = Math.max(1, Number(query.page) || 1)

  const { branch } = await requireBranch(branchId)
  const orders = await vendorAPI.listOrders(branchId, {
    status: filter.value,
    page,
  })

  const base = `/vendor/${branchId}/orders`
  const hrefFor = (status: OrderListFilter, p = 1) =>
    `${base}?status=${status}${p > 1 ? `&page=${p}` : ""}`

  return (
    <>
      <PageHeader
        title="Orders"
        description={
          filter.value === "active"
            ? `${orders.total_count} to do at ${branch.name}, soonest first`
            : branch.name
        }
      />
      {/* New orders must be accepted within minutes: live updates, with a
          10s reload as the fallback. */}
      {filter.value === "active" && <LiveOrders branchId={branchId} />}

      <FindByCode branchId={branchId} />

      <LinkTabs
        className="mb-4"
        tabs={FILTERS.map((f) => ({
          href: hrefFor(f.value),
          label: f.label,
          active: f.value === filter.value,
        }))}
      />

      {orders.data.length === 0 ? (
        <EmptyState
          icon={Inbox}
          title="Nothing here"
          description={filter.empty}
          className="rounded-xl border"
        />
      ) : (
        <OrderList orders={orders.data} branchId={branchId} />
      )}

      <SimplePagination
        page={orders.page}
        totalPages={orders.total_pages}
        hrefForPage={(p) => hrefFor(filter.value, p)}
      />
    </>
  )
}
