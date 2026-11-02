import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft, PartyPopper } from "lucide-react"

import { PageHeader } from "@/components/common/page-header"
import { Stepper } from "@/components/common/stepper"
import { OwnerInviteForm } from "@/components/admin/owner-invite-form"
import { setupStepper, type SetupStep } from "@/components/admin/setup-steps"
import {
  BranchStep,
  StoreDetailsStep,
} from "@/components/admin/setup-wizard-steps"
import { StorefrontKeyPanel } from "@/components/admin/storefront-key-panel"
import { SyncStatus } from "@/components/admin/sync-status"
import { SyncButton } from "@/components/portal/sync-button"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { triggerBranchSyncAction } from "@/lib/admin/actions"
import { adminAPI } from "@/lib/admin/api"
import { config } from "@/lib/config"

export const metadata: Metadata = { title: "Set up vendor" }

type Props = {
  params: Promise<{ vendorId: string }>
  searchParams: Promise<{ branch?: string; step?: string }>
}

const STEP_COPY: Record<
  Exclude<SetupStep, 0>,
  { title: string; description: string }
> = {
  1: {
    title: "Connect a branch",
    description:
      "The shop's inventory system, where its products and prices come from.",
  },
  2: {
    title: "Store details",
    description:
      "Contact details, collection and delivery, and opening hours customers will see.",
  },
  3: {
    title: "Invite the owner",
    description:
      "The owner signs in to the vendor portal to run orders and their store.",
  },
  4: {
    title: "Go live",
    description:
      "Give the branch's shop its key, then pull its products from its inventory system.",
  },
}

// Onboarding wizard. Which step to show is worked out from what's already
// set up, so the admin can leave at any point and come back to the same
// place. ?branch=<id> picks the branch to set up (?branch=new adds one);
// ?step=<n> revisits a finished step.
export default async function VendorSetupPage({ params, searchParams }: Props) {
  const { vendorId } = await params
  const query = await searchParams
  const vendor = await adminAPI.getVendor(vendorId)
  if (!vendor) notFound()

  const addingBranch = query.branch === "new"
  const branch = addingBranch
    ? null
    : (vendor.branches.find((b) => b.id === query.branch) ??
      vendor.branches[0] ??
      null)

  const done = [
    true,
    !!branch,
    !!branch?.has_store_details,
    vendor.users.some((u) => u.role === "owner"),
    !!branch?.storefront_api_key_prefix &&
      branch.last_sync_status === "success",
  ]
  const firstOpen = done.findIndex((d) => !d)
  const requested = Number(query.step)
  const step: SetupStep | null = addingBranch
    ? 1
    : requested >= 1 &&
        requested <= 4 &&
        (done[requested] || requested === firstOpen)
      ? (requested as SetupStep)
      : firstOpen === -1
        ? null
        : (firstOpen as SetupStep)

  const base = `/admin/vendors/${vendor.id}/setup`
  const withBranch = (id: string | undefined, step?: number) => {
    const p = new URLSearchParams()
    if (id) p.set("branch", id)
    if (step) p.set("step", String(step))
    const qs = p.toString()
    return qs ? `${base}?${qs}` : base
  }
  const nextHref = withBranch(branch?.id)

  // Only fetch what the current step needs.
  const [categories, branchDetail, storeInfo, syncStates] = await Promise.all([
    step === 1 ? adminAPI.listCategories() : null,
    step === 1 && branch ? adminAPI.getBranch(branch.id) : null,
    step === 2 && branch ? adminAPI.getStoreInfo(branch.id) : null,
    step === 4 && branch ? adminAPI.syncState(branch.id) : null,
  ])
  const productSync = syncStates?.find((s) => s.resource === "product")

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <Button
        variant="ghost"
        size="sm"
        className="-mb-4 -ml-2 w-fit"
        nativeButton={false}
        render={<Link href={`/admin/vendors/${vendor.id}`} />}
      >
        <ArrowLeft />
        {vendor.name}
      </Button>
      <PageHeader
        className="mb-0"
        title={`Set up ${vendor.name}`}
        description={
          branch && !addingBranch
            ? `Branch: ${branch.name}`
            : "Adding a new branch"
        }
      />
      <Stepper
        steps={setupStepper(done, step ?? 4, (s) => withBranch(branch?.id, s))}
      />

      {step === null ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-6 text-center">
            <PartyPopper className="size-8 text-primary" />
            <div className="text-lg font-semibold">{branch?.name} is live</div>
            <p className="max-w-md text-sm text-muted-foreground">
              It has its storefront key and products have synced. The owner can
              manage orders and store details from the vendor portal.
            </p>
            <div className="flex flex-wrap justify-center gap-2">
              <Button
                nativeButton={false}
                render={<Link href={`/admin/vendors/${vendor.id}`} />}
              >
                View vendor
              </Button>
              <Button
                variant="outline"
                nativeButton={false}
                render={<Link href={withBranch("new")} />}
              >
                Add another branch
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : step === 0 ? null : (
        <Card>
          <CardHeader>
            <CardTitle>{STEP_COPY[step].title}</CardTitle>
            <CardDescription>{STEP_COPY[step].description}</CardDescription>
          </CardHeader>
          <CardContent>
            {step === 1 && categories && (
              <BranchStep
                vendorId={vendor.id}
                branch={branchDetail ?? undefined}
                categories={categories}
                setupPath={base}
              />
            )}
            {step === 2 && branch && storeInfo && (
              <StoreDetailsStep
                branchId={branch.id}
                initial={storeInfo}
                nextHref={nextHref}
              />
            )}
            {step === 3 && (
              <OwnerInviteForm
                vendorId={vendor.id}
                vendorName={vendor.name}
                nextHref={nextHref}
              />
            )}
            {step === 4 && branch && (
              <div className="flex flex-col gap-6">
                <section className="flex flex-col gap-2">
                  <h3 className="font-medium">1. Storefront key</h3>
                  <StorefrontKeyPanel
                    branchId={branch.id}
                    keyPrefix={branch.storefront_api_key_prefix}
                    apiBaseUrl={config.api.baseUrl ?? ""}
                  />
                </section>
                <Separator />
                <section className="flex flex-col gap-3">
                  <h3 className="font-medium">2. Sync products</h3>
                  <SyncStatus
                    at={productSync?.last_run_at ?? null}
                    status={productSync?.last_run_status ?? null}
                    error={productSync?.last_run_error}
                  />
                  <SyncButton
                    branchId={branch.id}
                    syncAction={triggerBranchSyncAction}
                    label={
                      productSync?.last_run_at ? "Sync again" : "Run first sync"
                    }
                  />
                </section>
              </div>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  )
}
