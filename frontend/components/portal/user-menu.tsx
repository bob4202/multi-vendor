"use client"

import { useTransition } from "react"
import { LogOut } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

export interface PortalUser {
  name: string
  email: string
  // Short line under the name, e.g. "Owner" or "Platform admin".
  role: string
  // Extra line in the menu, e.g. the vendor's name.
  detail?: string
}

function initials(name: string) {
  return name
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase())
    .join("")
}

// Account menu for either portal: who's signed in, and sign out (the
// portal's logout server action).
function UserMenu({
  user,
  logoutAction,
  showName = false,
}: {
  user: PortalUser
  logoutAction: () => Promise<void>
  showName?: boolean
}) {
  const [pending, startTransition] = useTransition()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            className={
              showName
                ? "h-auto w-full justify-start gap-2.5 px-2 py-1.5"
                : undefined
            }
            size={showName ? "default" : "icon"}
            aria-label="Account"
          />
        }
      >
        <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
          {initials(user.name)}
        </span>
        {showName && (
          <span className="flex min-w-0 flex-col items-start text-left">
            <span className="truncate text-sm font-medium">{user.name}</span>
            <span className="truncate text-xs text-muted-foreground">
              {user.role}
            </span>
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex flex-col gap-0.5 py-1.5">
            <span className="text-sm font-medium text-foreground">
              {user.name}
            </span>
            <span className="font-normal">{user.email}</span>
            <span className="font-normal">
              {user.role}
              {user.detail && ` · ${user.detail}`}
            </span>
          </DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          variant="destructive"
          disabled={pending}
          onClick={() => startTransition(() => logoutAction())}
        >
          <LogOut />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export { UserMenu }
