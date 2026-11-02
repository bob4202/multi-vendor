"use client"

import { useState } from "react"
import { Check, Copy } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

// A read-only value with a copy button - invite links, API keys, env
// snippets. `multiline` shows it in a textarea instead of an input.
function CopyField({
  value,
  label,
  multiline = false,
  className,
}: {
  value: string
  label: string // for screen readers and the "copied" toast
  multiline?: boolean
  className?: string
}) {
  const [copied, setCopied] = useState(false)

  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      toast.success(`${label} copied`)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      toast.error("Couldn't copy - select the text and copy it manually")
    }
  }

  const field = multiline ? (
    <Textarea
      readOnly
      value={value}
      aria-label={label}
      onFocus={(e) => e.currentTarget.select()}
      className="font-mono text-xs"
    />
  ) : (
    <Input
      readOnly
      value={value}
      aria-label={label}
      onFocus={(e) => e.currentTarget.select()}
      className="font-mono text-xs"
    />
  )

  return (
    <div
      className={cn(
        "flex gap-2",
        multiline ? "items-start" : "items-center",
        className
      )}
    >
      <div className="min-w-0 flex-1">{field}</div>
      <Button
        type="button"
        variant="outline"
        size="icon"
        onClick={copy}
        aria-label={`Copy ${label}`}
      >
        {copied ? <Check /> : <Copy />}
      </Button>
    </div>
  )
}

export { CopyField }
