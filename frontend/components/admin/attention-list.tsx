import Link from "next/link"
import { CircleCheck } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { ATTENTION_KIND, groupAttention } from "@/lib/admin/attention"
import { cn } from "@/lib/utils"
import type { AttentionItem } from "@/types/admin"

const TONE = {
  critical: "bg-destructive/10 text-destructive",
  warning: "bg-amber-500/15 text-amber-700 dark:text-amber-400",
  info: "bg-muted text-muted-foreground",
}

// "Needs attention" on the overview: one row per vendor or branch, its
// problems, and a button to the place that fixes them.
function AttentionList({
  items,
  limit = 12,
}: {
  items: AttentionItem[]
  limit?: number
}) {
  const groups = groupAttention(items)

  if (groups.length === 0) {
    return (
      <p className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
        <CircleCheck className="size-4 text-emerald-600" />
        Nothing needs attention. Every branch is set up and syncing.
      </p>
    )
  }

  return (
    <>
      <ul className="flex flex-col divide-y">
        {groups.slice(0, limit).map((g) => (
          <li
            key={g.key}
            className="flex flex-col gap-2 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between"
          >
            <div className="flex min-w-0 flex-col gap-1.5">
              <span className="truncate font-medium">{g.title}</span>
              <div className="flex flex-wrap gap-1.5">
                {g.items.map((item, i) => (
                  <Badge
                    key={i}
                    title={item.detail}
                    className={cn(TONE[ATTENTION_KIND[item.kind].tone])}
                  >
                    {ATTENTION_KIND[item.kind].label}
                  </Badge>
                ))}
              </div>
              {g.items[0].kind === "sync_failed" && (
                <p className="line-clamp-2 text-xs text-muted-foreground">
                  {g.items[0].detail}
                </p>
              )}
            </div>
            <Button
              variant="outline"
              size="sm"
              className="w-fit shrink-0"
              nativeButton={false}
              render={<Link href={g.action.href} />}
            >
              {g.action.label}
            </Button>
          </li>
        ))}
      </ul>
      {groups.length > limit && (
        <p className="mt-3 text-sm text-muted-foreground">
          And {groups.length - limit} more. Open{" "}
          <Link href="/admin/vendors" className="underline underline-offset-4">
            Vendors
          </Link>{" "}
          to see everything.
        </p>
      )}
    </>
  )
}

export { AttentionList }
