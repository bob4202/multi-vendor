"use client"

import { useState, useTransition } from "react"
import { Loader2 } from "lucide-react"
import { toast } from "sonner"

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
import { updateVendorAction } from "@/lib/admin/actions"

// Deactivate (with confirmation - it signs everyone out) or reactivate a vendor.
function VendorStatusButton({
  vendorId,
  vendorName,
  isActive,
}: {
  vendorId: string
  vendorName: string
  isActive: boolean
}) {
  const [confirming, setConfirming] = useState(false)
  const [pending, startTransition] = useTransition()

  function setActive(active: boolean) {
    startTransition(async () => {
      const res = await updateVendorAction(vendorId, { is_active: active })
      setConfirming(false)
      if (res.ok) {
        toast.success(
          active ? `${vendorName} reactivated` : `${vendorName} deactivated`
        )
      } else {
        toast.error(res.error)
      }
    })
  }

  if (!isActive) {
    return (
      <Button
        variant="outline"
        disabled={pending}
        onClick={() => setActive(true)}
      >
        {pending && <Loader2 className="animate-spin" />}
        Reactivate
      </Button>
    )
  }

  return (
    <>
      <Button variant="destructive" onClick={() => setConfirming(true)}>
        Deactivate
      </Button>
      <AlertDialog
        open={confirming}
        onOpenChange={(open) => !pending && setConfirming(open)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Deactivate {vendorName}?</AlertDialogTitle>
            <AlertDialogDescription>
              Everyone at {vendorName} is signed out of the vendor portal
              straight away and can&apos;t sign back in until you reactivate it.
              Their branches, orders and settings are kept.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
            <Button
              variant="destructive"
              disabled={pending}
              onClick={() => setActive(false)}
            >
              {pending && <Loader2 className="animate-spin" />}
              Deactivate
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

export { VendorStatusButton }
