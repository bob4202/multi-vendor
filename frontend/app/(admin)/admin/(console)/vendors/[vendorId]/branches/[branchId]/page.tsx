import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft } from "lucide-react"

import { PageHeader } from "@/components/common/page-header"
import { BranchForm } from "@/components/admin/branch-form"
import { StorefrontKeyPanel } from "@/components/admin/storefront-key-panel"
import { SyncStatus } from "@/components/admin/sync-status"
import { StoreInfoForm } from "@/components/portal/store-info-form"
import { SyncButton } from "@/components/portal/sync-button"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  saveBranchStoreInfoAction,
  triggerBranchSyncAction,
} from "@/lib/admin/actions"
import { adminAPI } from "@/lib/admin/api"
import { config } from "@/lib/config"

export const metadata: Metadata = { title: "Branch" }

type Props = { params: Promise<{ vendorId: string; branchId: string }> }

export default async function AdminBranchPage({ params }: Props) {
  const { vendorId, branchId } = await params
  const [branch, categories, providers] = await Promise.all([
    adminAPI.getBranch(branchId),
    adminAPI.listCategories(),
    adminAPI.listInventoryProviders(),
  ])
  // The branch must belong to the vendor in the URL.
  if (!branch || branch.vendor_id !== vendorId) notFound()

  const [storeInfo, syncStates] = await Promise.all([
    adminAPI.getStoreInfo(branchId),
    adminAPI.syncState(branchId),
  ])
  const productSync = syncStates.find((s) => s.resource === "product")

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
      <Button
        variant="ghost"
        size="sm"
        className="-ml-2 w-fit"
        nativeButton={false}
        render={<Link href={`/admin/vendors/${vendorId}`} />}
      >
        <ArrowLeft />
        Back to vendor
      </Button>
      <PageHeader
        className="mb-2"
        title={branch.name}
        description={branch.host}
      />

      <Card>
        <CardHeader>
          <CardTitle>Products sync</CardTitle>
          <CardDescription>
            Products and prices come from the branch&apos;s inventory system.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <SyncStatus
            at={productSync?.last_run_at ?? null}
            status={productSync?.last_run_status ?? null}
            error={productSync?.last_run_error}
          />
          <SyncButton
            branchId={branchId}
            syncAction={triggerBranchSyncAction}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Storefront key</CardTitle>
          <CardDescription>
            What the branch&apos;s online shop uses to load its products.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <StorefrontKeyPanel
            branchId={branchId}
            keyPrefix={branch.storefront_api_key_prefix}
            apiBaseUrl={config.api.baseUrl ?? ""}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Inventory sync</CardTitle>
        </CardHeader>
        <CardContent>
          <BranchForm
            vendorId={vendorId}
            branch={branch}
            categories={categories}
            providers={providers}
          />
        </CardContent>
      </Card>

      <StoreInfoForm
        branchId={branchId}
        initial={storeInfo}
        saveAction={saveBranchStoreInfoAction}
      />
    </div>
  )
}
