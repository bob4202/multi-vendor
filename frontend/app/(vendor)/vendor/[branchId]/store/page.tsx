import type { Metadata } from "next"
import { Lock } from "lucide-react"

import { ClosuresEditor } from "@/components/vendor/closures-editor"
import { PageHeader } from "@/components/common/page-header"
import { StoreInfoForm } from "@/components/portal/store-info-form"
import { saveStoreInfoAction } from "@/lib/vendor/actions"
import { vendorAPI } from "@/lib/vendor/api"
import { requireBranch } from "@/lib/vendor/session"

export const metadata: Metadata = { title: "Store details" }

export default async function StorePage({
  params,
}: {
  params: Promise<{ branchId: string }>
}) {
  const { branchId } = await params
  const { session, branch } = await requireBranch(branchId)
  const [info, closures] = await Promise.all([
    vendorAPI.getStoreInfo(branchId),
    vendorAPI.listClosures(branchId),
  ])
  const readOnly = !session.permissions.edit_store_info

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
      <PageHeader
        className="mb-2"
        title="Store details"
        description={`What customers see for ${branch.name}.`}
      />
      {readOnly && (
        <p className="flex items-center gap-2 rounded-lg bg-muted px-3 py-2 text-sm text-muted-foreground">
          <Lock className="size-4" />
          Only owners and managers can change these details.
        </p>
      )}
      {/* key: a fresh form if the branch changes under the same route */}
      <StoreInfoForm
        key={branchId}
        branchId={branchId}
        initial={info}
        readOnly={readOnly}
        saveAction={saveStoreInfoAction}
      />
      <ClosuresEditor
        branchId={branchId}
        closures={closures}
        readOnly={readOnly}
      />
    </div>
  )
}
