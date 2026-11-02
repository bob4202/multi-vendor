"use client"

import { useState, useTransition } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { KeyRound, Loader2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { findOrderByCodeAction } from "@/lib/vendor/actions"
import type { OrderSummary } from "@/types/vendor"

// A customer at the counter reads out their code: find their order. One
// match opens it (with the hand-over ready if it's ready); a rare clash
// lists the matches by name.
function FindByCode({ branchId }: { branchId: string }) {
  const router = useRouter()
  const [code, setCode] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [matches, setMatches] = useState<OrderSummary[]>([])
  const [pending, startTransition] = useTransition()

  const href = (o: OrderSummary) =>
    `/vendor/${branchId}/orders/${o.id}?code=${code}`

  function find(e: React.FormEvent) {
    e.preventDefault()
    setMatches([])
    if (!/^\d{6}$/.test(code)) {
      setError("Codes are 6 digits.")
      return
    }
    startTransition(async () => {
      const res = await findOrderByCodeAction(branchId, code)
      if (!res.ok) {
        setError(res.error)
        return
      }
      if (res.data.length === 0) {
        setError("No open collection order has that code.")
      } else if (res.data.length === 1) {
        router.push(href(res.data[0]))
      } else {
        setMatches(res.data)
      }
    })
  }

  return (
    <div className="mb-4 flex flex-col gap-2">
      <form onSubmit={find} className="flex max-w-sm gap-2">
        <div className="relative flex-1">
          <KeyRound className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label="Collection code"
            placeholder="Find by collection code"
            inputMode="numeric"
            maxLength={6}
            className="pl-8 tabular-nums"
            value={code}
            onChange={(e) => {
              setCode(e.target.value.replace(/\D/g, ""))
              setError(null)
            }}
            aria-invalid={!!error || undefined}
          />
        </div>
        <Button type="submit" variant="outline" disabled={pending}>
          {pending && <Loader2 className="animate-spin" />}
          Find
        </Button>
      </form>
      {error && (
        <p className="animate-in text-xs text-destructive fade-in-0">
          {error}
        </p>
      )}
      {matches.length > 1 && (
        <div className="flex max-w-sm animate-in flex-col gap-1 rounded-lg border p-2 text-sm fade-in-0">
          <p className="px-1 text-xs text-muted-foreground">
            More than one order has this code - check the name:
          </p>
          {matches.map((o) => (
            <Link
              key={o.id}
              href={href(o)}
              className="flex justify-between gap-2 rounded-md px-1 py-1.5 hover:bg-muted"
            >
              <span className="font-medium">{o.reference}</span>
              <span className="truncate text-muted-foreground">
                {o.customer.name}
              </span>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}

export { FindByCode }
