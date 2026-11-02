import type { Metadata } from "next"
import Link from "next/link"
import { CircleAlert, MailWarning, Plus, Radio, Store } from "lucide-react"

import { PageHeader } from "@/components/common/page-header"
import { StatTile } from "@/components/common/stat-tile"
import { AttentionList } from "@/components/admin/attention-list"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { adminAPI } from "@/lib/admin/api"
import { formatDateTime } from "@/lib/format"

export const metadata: Metadata = { title: { absolute: "Overview | Admin" } }

export default async function AdminOverviewPage() {
  const { counts, attention, recent_activity } = await adminAPI.overview()

  return (
    <>
      <PageHeader
        title="Overview"
        description="How the platform is doing, and what needs someone to act."
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

      <div className="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile
          label="Active vendors"
          value={counts.active_vendors}
          icon={Store}
          href="/admin/vendors"
        />
        <StatTile
          label="Branches live"
          value={`${counts.live_branches} / ${counts.active_branches}`}
          hint="Storefront key issued and products synced"
          icon={Radio}
        />
        <StatTile
          label="Pending invites"
          value={counts.pending_invites}
          icon={MailWarning}
          tone={counts.pending_invites > 0 ? "warning" : "default"}
        />
        <StatTile
          label="Failed syncs"
          value={counts.failed_syncs}
          icon={CircleAlert}
          tone={counts.failed_syncs > 0 ? "warning" : "default"}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <Card className="min-w-0">
          <CardHeader>
            <CardTitle>Needs attention</CardTitle>
          </CardHeader>
          <CardContent>
            <AttentionList items={attention} />
          </CardContent>
        </Card>

        <Card className="min-w-0">
          <CardHeader>
            <CardTitle>Recent activity</CardTitle>
          </CardHeader>
          <CardContent>
            {recent_activity.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                Nothing yet. Admin actions show up here.
              </p>
            ) : (
              <ol className="flex flex-col gap-3">
                {recent_activity.map((a, i) => (
                  <li key={i} className="flex flex-col gap-0.5">
                    <span className="text-sm">{a.summary}</span>
                    <span className="text-xs text-muted-foreground">
                      {formatDateTime(a.created_at)} ·{" "}
                      {a.admin_name ?? "API key"}
                    </span>
                  </li>
                ))}
              </ol>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  )
}
