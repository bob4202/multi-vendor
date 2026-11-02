"use client"

import { useRouter } from "next/navigation"

import { BranchForm } from "@/components/admin/branch-form"
import { StoreInfoForm } from "@/components/portal/store-info-form"
import { saveBranchStoreInfoAction } from "@/lib/admin/actions"
import type { AdminBranch } from "@/types/admin"
import type { Category } from "@/types/category"
import type { StoreInfo } from "@/types/vendor"

// Client wrappers for the setup page's form steps: after a save they go back
// to the setup page, which works out the next step from what's now done.

function BranchStep({
  vendorId,
  branch,
  categories,
  setupPath,
}: {
  vendorId: string
  branch?: AdminBranch
  categories: Category[]
  setupPath: string // the setup page; the saved branch is added as ?branch=
}) {
  const router = useRouter()
  return (
    <BranchForm
      vendorId={vendorId}
      branch={branch}
      categories={categories}
      submitLabel="Save and continue"
      onSaved={(saved) => router.push(`${setupPath}?branch=${saved.id}`)}
    />
  )
}

function StoreDetailsStep({
  branchId,
  initial,
  nextHref,
}: {
  branchId: string
  initial: StoreInfo
  nextHref: string
}) {
  const router = useRouter()
  return (
    <StoreInfoForm
      branchId={branchId}
      initial={initial}
      saveAction={saveBranchStoreInfoAction}
      submitLabel="Save and continue"
      requireChanges={false}
      onSaved={() => router.push(nextHref)}
    />
  )
}

export { BranchStep, StoreDetailsStep }
