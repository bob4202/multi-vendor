import Link from "next/link"
import { Check } from "lucide-react"

import { cn } from "@/lib/utils"

export interface Step {
  label: string
  state: "done" | "current" | "upcoming"
  // Done steps can link back so they can be revisited.
  href?: string
}

// Progress through a multi-step flow. A vertical list on phones, a row of
// numbered steps from `md`.
function Stepper({ steps, className }: { steps: Step[]; className?: string }) {
  return (
    <ol
      className={cn(
        "flex flex-col gap-2 md:flex-row md:items-center md:gap-0",
        className
      )}
    >
      {steps.map((step, i) => {
        const marker = (
          <span
            className={cn(
              "flex size-7 shrink-0 items-center justify-center rounded-full border text-xs font-semibold transition-colors",
              step.state === "done" &&
                "border-primary bg-primary text-primary-foreground",
              step.state === "current" && "border-primary text-primary",
              step.state === "upcoming" && "text-muted-foreground"
            )}
          >
            {step.state === "done" ? <Check className="size-3.5" /> : i + 1}
          </span>
        )
        const label = (
          <span
            className={cn(
              "text-sm",
              step.state === "current" ? "font-medium" : "text-muted-foreground"
            )}
          >
            {step.label}
          </span>
        )
        return (
          <li
            key={step.label}
            aria-current={step.state === "current" ? "step" : undefined}
            className="flex items-center gap-2 md:flex-1 md:last:flex-none"
          >
            {step.href && step.state === "done" ? (
              <Link
                href={step.href}
                className="flex items-center gap-2 hover:underline"
              >
                {marker}
                {label}
              </Link>
            ) : (
              <span className="flex items-center gap-2">
                {marker}
                {label}
              </span>
            )}
            {i < steps.length - 1 && (
              <span
                aria-hidden
                className="mx-3 hidden h-px flex-1 bg-border md:block"
              />
            )}
          </li>
        )
      })}
    </ol>
  )
}

export { Stepper }
