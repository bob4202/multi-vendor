import Link from "next/link"
import type { LucideIcon } from "lucide-react"

import { cn } from "@/lib/utils"

// One headline number with a label, optionally linking to the detail.
// `tone="warning"` draws attention when the number needs action.
function StatTile({
  label,
  value,
  hint,
  icon: Icon,
  href,
  tone = "default",
}: {
  label: string
  value: number | string
  hint?: string
  icon: LucideIcon
  href?: string
  tone?: "default" | "warning"
}) {
  const body = (
    <>
      <div className="flex items-center justify-between gap-2 text-sm text-muted-foreground">
        {label}
        <Icon
          className={cn(
            "size-4",
            tone === "warning" && "text-amber-600 dark:text-amber-400"
          )}
        />
      </div>
      <div
        className={cn(
          "text-3xl font-semibold tracking-tight tabular-nums",
          tone === "warning" && "text-amber-700 dark:text-amber-400"
        )}
      >
        {value}
      </div>
      {hint && <div className="text-xs text-muted-foreground">{hint}</div>}
    </>
  )

  const className = "flex flex-col gap-1 rounded-xl border bg-card p-4"
  return href ? (
    <Link
      href={href}
      className={cn(className, "transition-colors hover:bg-muted/50")}
    >
      {body}
    </Link>
  ) : (
    <div className={className}>{body}</div>
  )
}

export { StatTile }
