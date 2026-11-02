"use client"

import { Minus, Plus, Trash2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

interface QuantityStepperProps {
  value: number
  onChange: (value: number) => void
  min?: number
  max?: number
  size?: "sm" | "default"
  // Swap the minus for a bin icon at the bottom of the range, to make it
  // obvious the next tap removes the item (cart lines, not pickers).
  removeAtMin?: boolean
  className?: string
}

// Controlled - the caller owns the number, so the same stepper works for a
// cart line (writes straight to the cart) and for a local "how many?"
// picker before adding.
function QuantityStepper({
  value,
  onChange,
  min = 0,
  max = 99,
  size = "default",
  removeAtMin = false,
  className,
}: QuantityStepperProps) {
  const buttonSize = size === "sm" ? "icon-sm" : "icon"
  const showRemove = removeAtMin && value <= min + 1

  return (
    <div
      className={cn(
        "inline-flex items-center gap-1 rounded-lg border border-border",
        size === "sm" ? "h-7" : "h-8",
        className
      )}
    >
      <Button
        type="button"
        variant="ghost"
        size={buttonSize}
        aria-label={showRemove ? "Remove item" : "Decrease quantity"}
        disabled={value <= min}
        onClick={() => onChange(value - 1)}
      >
        {showRemove ? <Trash2 /> : <Minus />}
      </Button>
      <span
        // Re-keying on value replays the tiny fade so the change registers.
        key={value}
        aria-live="polite"
        className="min-w-5 animate-in text-center text-sm font-medium tabular-nums duration-150 fade-in-0"
      >
        {value}
      </span>
      <Button
        type="button"
        variant="ghost"
        size={buttonSize}
        aria-label="Increase quantity"
        disabled={value >= max}
        onClick={() => onChange(value + 1)}
      >
        <Plus />
      </Button>
    </div>
  )
}

export { QuantityStepper }
