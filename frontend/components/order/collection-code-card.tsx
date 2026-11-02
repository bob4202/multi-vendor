import { Clock, KeyRound } from "lucide-react"

import { Card, CardContent } from "@/components/ui/card"
import { formatTime } from "@/lib/format"
import { cn } from "@/lib/utils"

// The code the customer shows at the counter, big enough to read from a
// phone held up to the till. Once ready, it also says when to collect by.
function CollectionCodeCard({
  code,
  collectBy,
  ready,
}: {
  code: string
  collectBy: string | null
  ready: boolean
}) {
  return (
    <Card
      className={cn(
        "animate-in duration-300 fade-in-0",
        ready && "ring-2 ring-emerald-500/40"
      )}
    >
      <CardContent className="flex flex-col items-center gap-2 text-center">
        <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
          <KeyRound className="size-4" />
          Collection code
        </p>
        <p className="font-mono text-4xl font-semibold tracking-[0.3em] tabular-nums">
          {code}
        </p>
        <p className="text-sm text-muted-foreground">
          Show this at the counter when you collect.
        </p>
        {ready && collectBy && (
          <p className="mt-1 flex items-center gap-1.5 rounded-full bg-emerald-500/10 px-3 py-1 text-sm font-medium text-emerald-700 dark:text-emerald-400">
            <Clock className="size-4" />
            Collect by {formatTime(collectBy)}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

export { CollectionCodeCard }
