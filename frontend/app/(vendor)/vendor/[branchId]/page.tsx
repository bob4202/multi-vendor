import { redirect } from "next/navigation"

export default async function BranchIndexPage({
  params,
}: {
  params: Promise<{ branchId: string }>
}) {
  const { branchId } = await params
  redirect(`/vendor/${branchId}/orders`)
}
