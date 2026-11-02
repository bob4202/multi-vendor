import { VendorShell } from "@/components/vendor/vendor-shell"
import { requireBranch } from "@/lib/vendor/session"

export default async function BranchLayout({
  children,
  params,
}: {
  children: React.ReactNode
  params: Promise<{ branchId: string }>
}) {
  const { branchId } = await params
  // Pages re-check with requireBranch too (layouts don't re-render on every
  // navigation), and the API checks again - this one picks the shell.
  const { session } = await requireBranch(branchId)

  return (
    <VendorShell session={session} branchId={branchId}>
      {children}
    </VendorShell>
  )
}
