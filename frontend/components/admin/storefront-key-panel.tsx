"use client"

import { useState, useTransition } from "react"
import { KeyRound, Loader2 } from "lucide-react"
import { toast } from "sonner"

import { CopyField } from "@/components/common/copy-field"
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
import { issueStorefrontKeyAction } from "@/lib/admin/actions"
import type { StorefrontKey } from "@/types/admin"

// Issues the key a branch's storefront deployment uses to call the API,
// and the env snippet to paste into that deployment. The key is shown once;
// replacing it immediately breaks the live shop until it's redeployed, so
// that asks first.
function StorefrontKeyPanel({
  branchId,
  keyPrefix,
  apiBaseUrl,
}: {
  branchId: string
  keyPrefix: string | null
  apiBaseUrl: string
}) {
  const [issued, setIssued] = useState<StorefrontKey | null>(null)
  const [confirming, setConfirming] = useState(false)
  const [pending, startTransition] = useTransition()

  function issue() {
    startTransition(async () => {
      const res = await issueStorefrontKeyAction(branchId)
      setConfirming(false)
      if (res.ok) {
        setIssued(res.data)
        toast.success("Storefront key issued")
      } else {
        toast.error(res.error)
      }
    })
  }

  if (issued) {
    const env = [
      `BASE_URL=${apiBaseUrl}`,
      `BRANCH_ID=${branchId}`,
      `STOREFRONT_KEY=${issued.api_key}`,
    ].join("\n")
    return (
      <div className="flex animate-in flex-col gap-3 duration-300 fade-in-0">
        <p className="text-sm">
          Put this in the branch&apos;s storefront deployment (
          <code className="font-mono text-xs">frontend/.env</code>) and redeploy
          it. <strong>The key won&apos;t be shown again.</strong>
        </p>
        <CopyField value={env} label="Storefront settings" multiline />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-3">
      {keyPrefix ? (
        <p className="flex items-center gap-2 text-sm">
          <KeyRound className="size-4 text-muted-foreground" />
          Key issued: <code className="font-mono text-xs">{keyPrefix}…</code>
        </p>
      ) : (
        <p className="text-sm text-muted-foreground">
          No key yet. The branch&apos;s shop can&apos;t load products until it
          has one.
        </p>
      )}
      <Button
        variant={keyPrefix ? "outline" : "default"}
        className="w-fit"
        disabled={pending}
        onClick={() => (keyPrefix ? setConfirming(true) : issue())}
      >
        {pending && <Loader2 className="animate-spin" />}
        {keyPrefix ? "Replace key" : "Issue storefront key"}
      </Button>

      <AlertDialog
        open={confirming}
        onOpenChange={(open) => !pending && setConfirming(open)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Replace the storefront key?</AlertDialogTitle>
            <AlertDialogDescription>
              The current key stops working straight away, so the live shop
              shows no products until you put the new key in its settings and
              redeploy. Only do this if the key leaked or was lost.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={pending}>
              Keep current key
            </AlertDialogCancel>
            <Button variant="destructive" disabled={pending} onClick={issue}>
              {pending && <Loader2 className="animate-spin" />}
              Replace key
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

export { StorefrontKeyPanel }
