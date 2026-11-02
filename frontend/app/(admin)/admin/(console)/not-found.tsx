import Link from "next/link"
import { SearchX } from "lucide-react"

import { EmptyState } from "@/components/common/empty-state"
import { Button } from "@/components/ui/button"

export default function AdminNotFound() {
  return (
    <EmptyState
      icon={SearchX}
      title="Not found"
      description="This vendor or branch doesn't exist. It may have been typed wrong."
      action={
        <Button nativeButton={false} render={<Link href="/admin/vendors" />}>
          All vendors
        </Button>
      }
    />
  )
}
