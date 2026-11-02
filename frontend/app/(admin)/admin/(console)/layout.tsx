import { AdminShell } from "@/components/admin/admin-shell"
import { requireAdmin } from "@/lib/admin/session"

// Everything behind the admin sign-in shares this shell.
export default async function AdminConsoleLayout({
  children,
}: {
  children: React.ReactNode
}) {
  const admin = await requireAdmin()
  return <AdminShell admin={admin}>{children}</AdminShell>
}
