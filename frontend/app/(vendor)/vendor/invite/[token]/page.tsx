import type { Metadata } from "next"
import Link from "next/link"
import { LinkIcon } from "lucide-react"

import { EmptyState } from "@/components/common/empty-state"
import { AcceptInviteForm } from "@/components/vendor/accept-invite-form"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { ApiError } from "@/lib/api-client"
import { acceptInviteAction } from "@/lib/vendor/actions"
import { vendorAPI } from "@/lib/vendor/api"

export const metadata: Metadata = { title: "Set up your account" }

type Props = { params: Promise<{ token: string }> }

// Where an invite link lands: choose a password, then straight into the
// portal. Reachable without a session (see proxy.ts).
export default async function AcceptInvitePage({ params }: Props) {
  const { token } = await params

  let invite = null
  try {
    invite = await vendorAPI.getInvite(token)
  } catch (err) {
    // 404 unknown / 410 used or expired: same advice either way.
    if (
      !(err instanceof ApiError) ||
      (err.status !== 404 && err.status !== 410)
    ) {
      throw err
    }
  }

  return (
    <div className="flex min-h-svh items-center justify-center bg-muted/30 px-4 py-12">
      <Card className="w-full max-w-sm animate-in duration-300 fade-in-0 zoom-in-95">
        {invite ? (
          <>
            <CardHeader>
              <CardTitle className="text-xl">Welcome, {invite.name}</CardTitle>
              <CardDescription>
                Choose a password for your {invite.vendor_name} account (
                {invite.email}).
              </CardDescription>
            </CardHeader>
            <CardContent>
              <AcceptInviteForm
                email={invite.email}
                action={acceptInviteAction.bind(null, token)}
              />
            </CardContent>
          </>
        ) : (
          <CardContent>
            <EmptyState
              icon={LinkIcon}
              title="This link doesn't work any more"
              description="Invite links work once and expire after 7 days. Ask whoever invited you for a new one."
              action={
                <Button
                  variant="outline"
                  nativeButton={false}
                  render={<Link href="/vendor/login" />}
                >
                  Go to sign in
                </Button>
              }
            />
          </CardContent>
        )}
      </Card>
    </div>
  )
}
