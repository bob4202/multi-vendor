import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

interface FormFieldProps {
  id: string
  label: string
  error?: string
  hint?: React.ReactNode
  optional?: boolean
  className?: string
  // The control itself (Input, Textarea, ...). Give it id={id} and
  // aria-invalid={!!error} so the label and error are wired up.
  children: React.ReactNode
}

// Label + control + hint/error, laid out the same everywhere.
function FormField({
  id,
  label,
  error,
  hint,
  optional,
  className,
  children,
}: FormFieldProps) {
  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <Label htmlFor={id}>
        {label}
        {optional && (
          <span className="font-normal text-muted-foreground">(optional)</span>
        )}
      </Label>
      {children}
      {error ? (
        <p
          id={`${id}-error`}
          className="animate-in text-xs text-destructive duration-150 fade-in-0"
        >
          {error}
        </p>
      ) : (
        hint && <p className="text-xs text-muted-foreground">{hint}</p>
      )}
    </div>
  )
}

export { FormField }
