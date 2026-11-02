"use client"

import { usePathname, useRouter } from "next/navigation"
import { Check, ChevronsUpDown, Store } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { VendorBranch } from "@/types/vendor"

// Switches branch while staying in the same section: /vendor/A/store ->
// /vendor/B/store. Deeper pages (an order) belong to one branch, so those
// land on the section's index instead.
function sectionPath(pathname: string, branchId: string) {
  const section = pathname.split("/")[3] ?? "orders"
  return `/vendor/${branchId}/${section}`
}

function BranchSwitcher({
  branches,
  currentId,
}: {
  branches: VendorBranch[]
  currentId: string
}) {
  const router = useRouter()
  const pathname = usePathname()
  const current = branches.find((b) => b.id === currentId)

  if (branches.length <= 1) {
    return (
      <div className="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium">
        <Store className="size-4 text-muted-foreground" />
        <span className="truncate">{current?.name}</span>
      </div>
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="outline"
            className="h-auto w-full justify-between px-3 py-2"
          />
        }
      >
        <span className="flex min-w-0 items-center gap-2">
          <Store className="size-4 text-muted-foreground" />
          <span className="truncate">{current?.name}</span>
        </span>
        <ChevronsUpDown className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent className="min-w-56">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Your branches</DropdownMenuLabel>
          {branches.map((b) => (
            <DropdownMenuItem
              key={b.id}
              onClick={() => router.push(sectionPath(pathname, b.id))}
            >
              <span className="flex-1 truncate">{b.name}</span>
              {!b.is_active && (
                <span className="text-xs text-muted-foreground">Inactive</span>
              )}
              {b.id === currentId && <Check />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export { BranchSwitcher }
