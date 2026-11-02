import type { Metadata } from "next"
import { redirect } from "next/navigation"

import { LoginForm } from "@/components/portal/login-form"
import { loginAction } from "@/lib/vendor/actions"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { safeVendorRedirect } from "@/lib/vendor/constants"
import { getSession } from "@/lib/vendor/session"

export const metadata: Metadata = { title: "Sign in" }

type Props = {
  searchParams: Promise<{ next?: string; expired?: string }>
}

export default async function VendorLoginPage({ searchParams }: Props) {
  const { next, expired } = await searchParams

  // Already signed in (and the session is still valid): skip the form.
  if (await getSession()) {
    redirect(safeVendorRedirect(next))
  }

  return (
    <div className="flex min-h-svh items-center justify-center bg-muted/30 px-4 py-12">
      <Card className="w-full max-w-sm animate-in duration-300 fade-in-0 zoom-in-95">
        <CardHeader>
          <CardTitle className="text-xl">Grocery Shop vendor portal</CardTitle>
          <CardDescription>
            Sign in to manage your branch&apos;s orders and store details.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <LoginForm
            action={loginAction}
            next={next}
            notice={
              expired
                ? "Your session has ended. Please sign in again."
                : undefined
            }
          />
        </CardContent>
      </Card>
    </div>
  )
}
