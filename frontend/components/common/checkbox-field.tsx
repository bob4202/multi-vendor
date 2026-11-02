"use client"

import { Checkbox } from "@/components/ui/checkbox"
import { cn } from "@/lib/utils"

interface CheckboxFieldProps {
  id: string
  // Submitted as "on" when ticked, left out otherwise (like a native box).
  name: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  label: React.ReactNode
  description?: React.ReactNode
  error?: string
  className?: string
}

// A checkbox with its label, an optional explanation and an error, for
// options and agreements inside forms.
function CheckboxField({
  id,
  name,
  checked,
  onCheckedChange,
  label,
  description,
  error,
  className,
}: CheckboxFieldProps) {
  return (
    <div className={cn("flex items-start gap-3", className)}>
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(v) => onCheckedChange(v === true)}
        aria-invalid={!!error || undefined}
        aria-describedby={description || error ? `${id}-help` : undefined}
        className="mt-0.5"
      />
      {checked && <input type="hidden" name={name} value="on" />}
      <div className="flex min-w-0 flex-col gap-1">
        <label htmlFor={id} className="text-sm leading-snug font-medium">
          {label}
        </label>
        {error ? (
          <p
            id={`${id}-help`}
            className="animate-in text-xs text-destructive duration-150 fade-in-0"
          >
            {error}
          </p>
        ) : (
          description && (
            <p id={`${id}-help`} className="text-xs text-muted-foreground">
              {description}
            </p>
          )
        )}
      </div>
    </div>
  )
}

export { CheckboxField }
