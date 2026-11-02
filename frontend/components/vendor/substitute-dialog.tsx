"use client"

import { useEffect, useState, useTransition } from "react"
import { Check, Loader2, Search } from "lucide-react"
import { toast } from "sonner"

import { QuantityStepper } from "@/components/cart/quantity-stepper"
import { Price } from "@/components/common/price"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  markItemUnavailableAction,
  searchProductsAction,
} from "@/lib/vendor/actions"
import { cn } from "@/lib/utils"
import type { Product } from "@/types/product"
import type { OrderItem } from "@/types/vendor"

// "Out of stock" for one order line: remove it, or offer the customer a
// similar product. The customer must accept the offer, and never pays more
// than the line they ordered.
function SubstituteDialog({
  branchId,
  orderId,
  item,
  allowSubstitutions,
  open,
  onOpenChange,
}: {
  branchId: string
  orderId: string
  item: OrderItem
  allowSubstitutions: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [search, setSearch] = useState("")
  const [results, setResults] = useState<Product[]>([])
  const [searching, setSearching] = useState(false)
  const [picked, setPicked] = useState<Product | null>(null)
  const [quantity, setQuantity] = useState(item.quantity)
  const [pending, startTransition] = useTransition()
  const [action, setAction] = useState<"remove" | "offer" | null>(null)

  // Debounced product search.
  useEffect(() => {
    if (!open || !allowSubstitutions) return
    const q = search.trim()
    if (q.length < 2) {
      setResults([])
      return
    }
    setSearching(true)
    const t = setTimeout(async () => {
      const res = await searchProductsAction(branchId, q)
      setResults(
        res.ok
          ? res.data.filter(
              (p) =>
                p.id !== item.product_id &&
                p.price !== null &&
                p.can_be_sold !== false
            )
          : []
      )
      setSearching(false)
    }, 300)
    return () => clearTimeout(t)
  }, [search, open, allowSubstitutions, branchId, item.product_id])

  function submit(substitute: { product_id: string; quantity: number } | null) {
    setAction(substitute ? "offer" : "remove")
    startTransition(async () => {
      const res = await markItemUnavailableAction(
        branchId,
        orderId,
        item.id,
        substitute
      )
      if (res.ok) {
        toast.success(
          substitute
            ? "Substitute offered - waiting for the customer"
            : `${item.name} removed`
        )
        onOpenChange(false)
        return
      }
      toast.error(res.issues[0]?.message ?? res.error)
    })
  }

  const normal = picked?.price != null ? picked.price * quantity : null
  const charged = normal === null ? null : Math.min(item.line_total, normal)

  return (
    <Dialog open={open} onOpenChange={(o) => !pending && onOpenChange(o)}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {item.quantity} × {item.name} is out of stock
          </DialogTitle>
          <DialogDescription>
            {allowSubstitutions
              ? "Offer something similar - the customer has to accept it - or remove the item."
              : "The customer asked for no substitutes, so the item will be removed."}
          </DialogDescription>
        </DialogHeader>

        {allowSubstitutions && (
          <div className="flex min-w-0 flex-col gap-3">
            <div className="relative">
              <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                autoFocus
                className="pl-8"
                placeholder="Search for a substitute"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
            <ul className="flex max-h-56 flex-col overflow-y-auto rounded-lg border">
              {searching && (
                <li className="flex items-center gap-2 p-3 text-sm text-muted-foreground">
                  <Loader2 className="size-4 animate-spin" />
                  Searching…
                </li>
              )}
              {!searching && results.length === 0 && (
                <li className="p-3 text-sm text-muted-foreground">
                  {search.trim().length < 2
                    ? "Type at least 2 letters."
                    : "No matching products."}
                </li>
              )}
              {!searching &&
                results.map((p) => (
                  <li key={p.id}>
                    <button
                      type="button"
                      onClick={() => setPicked(p)}
                      className={cn(
                        "flex w-full items-center justify-between gap-3 px-3 py-2 text-left text-sm transition-colors hover:bg-muted",
                        picked?.id === p.id && "bg-muted"
                      )}
                    >
                      <span className="flex min-w-0 items-center gap-2">
                        {picked?.id === p.id && (
                          <Check className="size-4 shrink-0" />
                        )}
                        <span className="truncate">{p.name ?? "Product"}</span>
                      </span>
                      <Price
                        pence={p.price}
                        className="shrink-0 text-muted-foreground"
                      />
                    </button>
                  </li>
                ))}
            </ul>

            {picked && (
              <div className="flex animate-in flex-col gap-2 rounded-lg bg-muted p-3 text-sm duration-200 fade-in-0">
                <div className="flex items-center justify-between gap-3">
                  <span className="min-w-0 truncate font-medium">
                    {picked.name}
                  </span>
                  <QuantityStepper
                    value={quantity}
                    onChange={setQuantity}
                    min={1}
                    size="sm"
                  />
                </div>
                <p>
                  Customer pays{" "}
                  <Price pence={charged} className="font-semibold" />
                  {normal !== null && charged !== null && normal > charged && (
                    <span className="text-muted-foreground">
                      {" "}
                      (normally <Price pence={normal} /> - the shop covers{" "}
                      <Price pence={normal - charged} />)
                    </span>
                  )}
                </p>
              </div>
            )}
          </div>
        )}

        <DialogFooter>
          <Button
            variant={allowSubstitutions ? "outline" : "destructive"}
            disabled={pending}
            onClick={() => submit(null)}
          >
            {pending && action === "remove" && (
              <Loader2 className="animate-spin" />
            )}
            Remove item
          </Button>
          {allowSubstitutions && (
            <Button
              disabled={pending || !picked}
              onClick={() =>
                picked && submit({ product_id: picked.id, quantity })
              }
            >
              {pending && action === "offer" && (
                <Loader2 className="animate-spin" />
              )}
              Offer substitute
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export { SubstituteDialog }
