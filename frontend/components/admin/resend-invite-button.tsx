"use client"

import { useState, useTransition } from "react"
import { Loader2 } from "lucide-react"
import { toast } from "sonner"

import { InviteLinkPanel } from "@/components/admin/invite-link-panel"
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { reissueInviteAction, type InviteLink } from "@/lib/admin/actions"

// Makes a new invite link for someone who hasn't set a password yet (any
// older link stops working) and shows it in a dialog to copy.
function ResendInviteButton({
  vendorId,
  vendorName,
  userId,
  userName,
}: {
  vendorId: string
  vendorName: string
  userId: string
  userName: string
}) {
  const [invite, setInvite] = useState<InviteLink | null>(null)
  const [pending, startTransition] = useTransition()

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        disabled={pending}
        onClick={() =>
          startTransition(async () => {
            const res = await reissueInviteAction(vendorId, userId)
            if (res.ok) setInvite(res.data)
            else toast.error(res.error)
          })
        }
      >
        {pending && <Loader2 className="animate-spin" />}
        New invite link
      </Button>
      <AlertDialog
        open={invite !== null}
        onOpenChange={(open) => !open && setInvite(null)}
      >
        <AlertDialogContent className="data-[size=default]:sm:max-w-lg">
          <AlertDialogHeader>
            <AlertDialogTitle>New invite for {userName}</AlertDialogTitle>
            <AlertDialogDescription>
              Any earlier link for {userName} no longer works.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {invite && (
            <InviteLinkPanel
              invite={invite}
              name={userName}
              vendorName={vendorName}
            />
          )}
          <AlertDialogFooter>
            <AlertDialogCancel>Done</AlertDialogCancel>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

export { ResendInviteButton }
