import Link from "next/link"
import { SearchX } from "lucide-react"

import { EmptyState } from "@/components/common/empty-state"
import { Button } from "@/components/ui/button"

export default function VendorNotFound() {
  return (
    <div className="flex min-h-svh items-center justify-center px-4">
      <EmptyState
        icon={SearchX}
        title="Not found"
        description="This page doesn't exist, or you don't have access to it."
        action={
          <Button nativeButton={false} render={<Link href="/vendor" />}>
            Back to your branches
          </Button>
        }
      />
    </div>
  )
}
