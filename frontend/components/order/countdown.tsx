"use client"

import { useEffect, useState } from "react"

import { cn } from "@/lib/utils"

interface CountdownProps {
  to: string // ISO timestamp
  className?: string
  // Shown once time is up (the server will flip the status shortly after).
  expiredLabel?: string
}

function remainingMs(to: string) {
  return Math.max(0, new Date(to).getTime() - Date.now())
}

// "4:32" ticking down to a deadline. Renders nothing on the server (the time
// would be stale by the time it reached the browser) to avoid a mismatch.
function Countdown({ to, className, expiredLabel = "0:00" }: CountdownProps) {
  const [ms, setMs] = useState<number | null>(null)

  useEffect(() => {
    setMs(remainingMs(to))
    const timer = setInterval(() => setMs(remainingMs(to)), 1000)
    return () => clearInterval(timer)
  }, [to])

  if (ms === null) return null
  const totalSeconds = Math.ceil(ms / 1000)
  const label =
    totalSeconds === 0
      ? expiredLabel
      : `${Math.floor(totalSeconds / 60)}:${String(totalSeconds % 60).padStart(2, "0")}`

  return (
    <span
      className={cn(
        "tabular-nums",
        totalSeconds <= 60 && totalSeconds > 0 && "text-destructive",
        className
      )}
    >
      {label}
    </span>
  )
}

export { Countdown }
