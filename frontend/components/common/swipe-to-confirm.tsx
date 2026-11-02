"use client"

import { useRef, useState } from "react"
import { ChevronsRight, Loader2 } from "lucide-react"

import { cn } from "@/lib/utils"

// Thumb is size-12 inside a p-1 track.
const THUMB = 48
const INSET = 4
// How far (as a share of the track) the thumb must travel to confirm.
const THRESHOLD = 0.85

interface SwipeToConfirmProps {
  label: string
  // Return false to spring back instead (e.g. the form isn't valid yet).
  onConfirm: () => boolean | void
  // Set while the confirmed action runs. When it goes back to false the
  // thumb springs back, so a failed attempt can be swiped again.
  loading?: boolean
  disabled?: boolean
  className?: string
}

// Drag the thumb to the end to confirm - for actions on phones where a
// stray tap shouldn't count. Let go early and it springs back. Keyboard
// users press Enter or Space on the thumb instead.
function SwipeToConfirm({
  label,
  onConfirm,
  loading = false,
  disabled = false,
  className,
}: SwipeToConfirmProps) {
  const trackRef = useRef<HTMLDivElement>(null)
  const startX = useRef(0)
  const [x, setX] = useState(0)
  // Track width minus the thumb, measured when a drag starts.
  const [travel, setTravel] = useState(0)
  const [dragging, setDragging] = useState(false)
  const [confirmed, setConfirmed] = useState(false)
  const [wasLoading, setWasLoading] = useState(loading)
  if (loading !== wasLoading) {
    setWasLoading(loading)
    if (!loading) {
      setConfirmed(false)
      setX(0)
    }
  }

  const maxX = () =>
    Math.max(0, (trackRef.current?.clientWidth ?? 0) - THUMB - INSET * 2)

  const confirm = () => {
    if (confirmed || disabled) return
    if (onConfirm() === false) {
      setX(0)
      return
    }
    setX(maxX())
    setConfirmed(true)
  }

  const progress = confirmed ? 1 : travel ? Math.min(1, x / travel) : 0

  return (
    <div
      ref={trackRef}
      className={cn(
        "relative h-14 overflow-hidden rounded-full bg-primary p-1 text-primary-foreground select-none",
        disabled && "opacity-50",
        className
      )}
    >
      {/* Fills in behind the thumb as it moves. */}
      <div
        aria-hidden
        className={cn(
          "absolute inset-y-0 left-0 rounded-full bg-primary-foreground/15",
          !dragging && "transition-[width] duration-200"
        )}
        style={{ width: x + THUMB + INSET * 2 }}
      />
      <span
        aria-hidden
        className={cn(
          "absolute inset-0 flex items-center justify-center pl-12 text-sm font-medium",
          !dragging && "transition-opacity duration-200"
        )}
        style={{ opacity: 1 - progress }}
      >
        {label}
      </span>
      <button
        type="button"
        aria-label={`${label} (press Enter)`}
        disabled={disabled || confirmed}
        className={cn(
          "relative grid size-12 touch-none place-items-center rounded-full bg-background text-foreground shadow-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50",
          !confirmed && "cursor-grab active:cursor-grabbing",
          !dragging && "transition-transform duration-200"
        )}
        style={{ transform: `translateX(${x}px)` }}
        onPointerDown={(event) => {
          if (confirmed || disabled) return
          event.currentTarget.setPointerCapture(event.pointerId)
          startX.current = event.clientX - x
          setTravel(maxX())
          setDragging(true)
        }}
        onPointerMove={(event) => {
          if (!dragging) return
          setX(Math.min(travel, Math.max(0, event.clientX - startX.current)))
        }}
        onPointerUp={() => {
          if (!dragging) return
          setDragging(false)
          if (travel > 0 && x >= travel * THRESHOLD) confirm()
          else setX(0)
        }}
        onPointerCancel={() => {
          setDragging(false)
          setX(0)
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault()
            confirm()
          }
        }}
      >
        {confirmed ? (
          <Loader2 className="size-5 animate-spin" />
        ) : (
          <ChevronsRight className="size-5" />
        )}
      </button>
    </div>
  )
}

export { SwipeToConfirm }
