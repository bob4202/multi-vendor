"use client"

import { useTransition } from "react"
import { useRouter } from "next/navigation"
import { Loader2, RefreshCw } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import type { ActionResult } from "@/types/api"

// Starts a product sync from the inventory system. The sync runs in the background, so
// the page refreshes a little later to pick up the new sync status.
function SyncButton({
  branchId,
  syncAction,
  label = "Sync now",
}: {
  branchId: string
  syncAction: (branchId: string) => Promise<ActionResult<void>>
  label?: string
}) {
  const router = useRouter()
  const [pending, startTransition] = useTransition()

  return (
    <Button
      variant="outline"
      disabled={pending}
      onClick={() =>
        startTransition(async () => {
          const res = await syncAction(branchId)
          if (!res.ok) {
            toast.error(res.error)
            return
          }
          toast.success("Sync started. Products update in a minute or two.")
          window.setTimeout(() => router.refresh(), 5_000)
        })
      }
    >
      {pending ? <Loader2 className="animate-spin" /> : <RefreshCw />}
      {label}
    </Button>
  )
}

export { SyncButton }
