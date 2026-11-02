import { CircleAlert, CircleCheck, CircleDashed } from "lucide-react"

import { formatDateTime } from "@/lib/format"

// Last product sync for a branch, in one line (plus the error if it failed).
function SyncStatus({
  at,
  status,
  error,
}: {
  at: string | null
  status: "success" | "error" | null
  error?: string | null
}) {
  if (!at || !status) {
    return (
      <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
        <CircleDashed className="size-4" />
        Never synced
      </span>
    )
  }
  if (status === "error") {
    return (
      <span className="flex flex-col gap-0.5 text-sm">
        <span className="flex items-center gap-1.5">
          <CircleAlert className="size-4 text-destructive" />
          Sync failed {formatDateTime(at)}
        </span>
        {error && (
          <span className="line-clamp-2 text-xs text-muted-foreground">
            {error}
          </span>
        )}
      </span>
    )
  }
  return (
    <span className="flex items-center gap-1.5 text-sm">
      <CircleCheck className="size-4 text-emerald-600" />
      Synced {formatDateTime(at)}
    </span>
  )
}

export { SyncStatus }
