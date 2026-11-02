"use client"

import { useEffect, useState } from "react"
import { Clock } from "lucide-react"

import { formatTime } from "@/lib/format"
import { cn } from "@/lib/utils"

// "Collect by 15:00" on a ready collection order, turning into "Overdue"
// once the time passes (it becomes a no-show shortly after).
function CollectByBadge({
  collectBy,
  className,
}: {
  collectBy: string
  className?: string
}) {
  // Decided after mount, so server and browser render the same thing.
  const [overdue, setOverdue] = useState(false)
  useEffect(() => {
    const check = () => setOverdue(Date.now() > new Date(collectBy).getTime())
    check()
    const t = window.setInterval(check, 30_000)
    return () => window.clearInterval(t)
  }, [collectBy])

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 text-xs font-medium",
        overdue
          ? "text-destructive"
          : "text-violet-700 dark:text-violet-400",
        className
      )}
    >
      <Clock className="size-3.5" />
      {overdue ? "Overdue · was due " : "Collect by "}
      {formatTime(collectBy)}
    </span>
  )
}

export { CollectByBadge }
