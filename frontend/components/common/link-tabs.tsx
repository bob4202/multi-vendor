import Link from "next/link"

import { cn } from "@/lib/utils"

export interface LinkTab {
  href: string
  label: string
  active: boolean
}

// Tabs whose state lives in the URL (?status=...), so a filter survives a
// refresh and can be shared. Styled like the shadcn tabs list.
function LinkTabs({
  tabs,
  className,
}: {
  tabs: LinkTab[]
  className?: string
}) {
  return (
    <div
      className={cn(
        "inline-flex w-fit max-w-full items-center gap-1 overflow-x-auto rounded-lg bg-muted p-1",
        className
      )}
    >
      {tabs.map((tab) => (
        <Link
          key={tab.href}
          href={tab.href}
          aria-current={tab.active ? "page" : undefined}
          className={cn(
            "rounded-md px-3 py-1 text-sm font-medium whitespace-nowrap transition-colors",
            tab.active
              ? "bg-background text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground"
          )}
        >
          {tab.label}
        </Link>
      ))}
    </div>
  )
}

export { LinkTabs }
