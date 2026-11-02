import { CopyField } from "@/components/common/copy-field"
import { formatDateTime } from "@/lib/format"
import type { InviteLink } from "@/lib/admin/actions"

// Shows a fresh invite link (only ever visible once) and a ready-to-send
// message, for pasting into WhatsApp, email, or a text.
function InviteLinkPanel({
  invite,
  name,
  vendorName,
}: {
  invite: InviteLink
  name: string
  vendorName: string
}) {
  const firstName = name.trim().split(/\s+/)[0] || "there"
  const message =
    `Hi ${firstName}, here's your link to set up your Grocery Shop vendor ` +
    `account for ${vendorName}. Open it and choose a password:\n${invite.url}\n` +
    `(The link works once and expires ${formatDateTime(invite.expires_at)}.)`

  return (
    <div className="flex animate-in flex-col gap-4 duration-300 fade-in-0">
      <div className="flex flex-col gap-1.5">
        <span className="text-sm font-medium">Invite link</span>
        <CopyField value={invite.url} label="Invite link" />
        <span className="text-xs text-muted-foreground">
          Works once, until {formatDateTime(invite.expires_at)}. It won&apos;t
          be shown again; you can always make a new one from the vendor&apos;s
          page.
        </span>
      </div>
      <div className="flex flex-col gap-1.5">
        <span className="text-sm font-medium">Message to send</span>
        <CopyField value={message} label="Message" multiline />
      </div>
    </div>
  )
}

export { InviteLinkPanel }
