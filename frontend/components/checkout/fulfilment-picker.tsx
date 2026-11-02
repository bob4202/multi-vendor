"use client"

import { Store, Truck } from "lucide-react"

import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { cn } from "@/lib/utils"
import type { FulfilmentType } from "@/types/order"

interface FulfilmentOption {
  value: FulfilmentType
  title: string
  description: string
  disabled?: boolean
}

interface FulfilmentPickerProps {
  value: FulfilmentType
  onChange: (value: FulfilmentType) => void
  options: FulfilmentOption[]
  name?: string
  error?: string
}

const ICONS = { collection: Store, delivery: Truck }

// Collection vs delivery as two selectable cards.
function FulfilmentPicker({
  value,
  onChange,
  options,
  name = "fulfilment_type",
  error,
}: FulfilmentPickerProps) {
  return (
    <div className="flex flex-col gap-2">
      <RadioGroup
        value={value}
        onValueChange={(v) => onChange(v as FulfilmentType)}
        className="grid gap-3 sm:grid-cols-2"
        aria-invalid={!!error}
      >
        {options.map((option) => {
          const Icon = ICONS[option.value]
          const id = `${name}-${option.value}`
          return (
            <Label
              key={option.value}
              htmlFor={id}
              className={cn(
                "flex cursor-pointer items-start gap-3 rounded-lg border p-3 font-normal transition-colors has-data-checked:border-primary has-data-checked:bg-primary/5",
                option.disabled && "cursor-not-allowed opacity-50"
              )}
            >
              <RadioGroupItem
                id={id}
                value={option.value}
                disabled={option.disabled}
                className="mt-0.5"
              />
              <div className="flex min-w-0 flex-col gap-0.5">
                <span className="flex items-center gap-1.5 font-medium">
                  <Icon className="size-4" />
                  {option.title}
                </span>
                <span className="text-xs text-muted-foreground">
                  {option.description}
                </span>
              </div>
            </Label>
          )
        })}
      </RadioGroup>
      <input type="hidden" name={name} value={value} />
      {error && <p className="text-xs text-destructive">{error}</p>}
    </div>
  )
}

export { FulfilmentPicker }
export type { FulfilmentOption }
