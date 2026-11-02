import type { AttentionItem, AttentionKind } from "@/types/admin"

// Turns the overview's flat attention list into one entry per vendor or
// branch, each with the single most useful next step.

interface KindMeta {
  label: string
  // Lower = more urgent. Matches the API's ordering.
  rank: number
  tone: "critical" | "warning" | "info"
}

export const ATTENTION_KIND: Record<AttentionKind, KindMeta> = {
  sync_failed: { label: "Sync failed", rank: 0, tone: "critical" },
  account_locked: { label: "Locked out", rank: 1, tone: "critical" },
  vendor_without_owner: { label: "No owner", rank: 2, tone: "warning" },
  invite_expired: { label: "Invite expired", rank: 3, tone: "warning" },
  invite_pending: { label: "Invite pending", rank: 4, tone: "info" },
  no_store_details: { label: "No store details", rank: 5, tone: "warning" },
  no_storefront_key: { label: "No storefront key", rank: 6, tone: "warning" },
  never_synced: { label: "Never synced", rank: 7, tone: "info" },
}

export interface AttentionGroup {
  key: string
  title: string // "Vendor" or "Vendor · Branch"
  items: AttentionItem[]
  action: { label: string; href: string }
}

function actionFor(top: AttentionItem): AttentionGroup["action"] {
  const vendor = `/admin/vendors/${top.vendor_id}`
  switch (top.kind) {
    case "sync_failed":
      return {
        label: "Check branch",
        href: `${vendor}/branches/${top.branch_id}`,
      }
    case "vendor_without_owner":
      return { label: "Invite owner", href: `${vendor}/setup` }
    case "no_store_details":
    case "no_storefront_key":
    case "never_synced":
      return {
        label: "Finish setup",
        href: `${vendor}/setup?branch=${top.branch_id}`,
      }
    default:
      return { label: "Open vendor", href: vendor }
  }
}

export function groupAttention(items: AttentionItem[]): AttentionGroup[] {
  const groups = new Map<string, AttentionItem[]>()
  for (const item of items) {
    const key = `${item.vendor_id}:${item.branch_id ?? ""}`
    groups.set(key, [...(groups.get(key) ?? []), item])
  }

  return [...groups.entries()]
    .map(([key, list]) => {
      const sorted = [...list].sort(
        (a, b) => ATTENTION_KIND[a.kind].rank - ATTENTION_KIND[b.kind].rank
      )
      const top = sorted[0]
      return {
        key,
        title: top.branch_name
          ? `${top.vendor_name} · ${top.branch_name}`
          : top.vendor_name,
        items: sorted,
        action: actionFor(top),
      }
    })
    .sort(
      (a, b) =>
        ATTENTION_KIND[a.items[0].kind].rank -
        ATTENTION_KIND[b.items[0].kind].rank
    )
}
