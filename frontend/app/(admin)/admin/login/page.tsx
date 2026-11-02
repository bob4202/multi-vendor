import type { Metadata } from "next"
import { redirect } from "next/navigation"

import { LoginForm } from "@/components/portal/login-form"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { adminLoginAction } from "@/lib/admin/actions"
import { safeAdminRedirect } from "@/lib/admin/constants"
import { getAdmin } from "@/lib/admin/session"

export const metadata: Metadata = { title: "Sign in" }

type Props = {
  searchParams: Promise<{ next?: string; expired?: string }>
}

export default async function AdminLoginPage({ searchParams }: Props) {
  const { next, expired } = await searchParams

  if (await getAdmin()) {
    redirect(safeAdminRedirect(next))
  }

  return (
    <div className="flex min-h-svh items-center justify-center bg-muted/30 px-4 py-12">
      <Card className="w-full max-w-sm animate-in duration-300 fade-in-0 zoom-in-95">
        <CardHeader>
          <CardTitle className="text-xl">Grocery Shop admin</CardTitle>
          <CardDescription>
            For the platform team: onboard vendors and keep branches running.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <LoginForm
            action={adminLoginAction}
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
