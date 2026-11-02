import Link from "next/link"
import { UserRound } from "lucide-react"

import { Button } from "@/components/ui/button"

// Account link in the header. It doesn't check who's signed in, so pages
// stay cacheable; /account sends guests to sign in.
function NavAccount() {
  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label="Your account"
      nativeButton={false}
      render={<Link href="/account" />}
    >
      <UserRound />
    </Button>
  )
}

export { NavAccount }
