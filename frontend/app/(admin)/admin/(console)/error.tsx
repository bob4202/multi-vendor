"use client"

import { useEffect } from "react"
import { CircleAlert } from "lucide-react"

import { EmptyState } from "@/components/common/empty-state"
import { Button } from "@/components/ui/button"

export default function AdminError({
  error,
  retry,
}: {
  error: Error & { digest?: string }
  retry: () => void
}) {
  useEffect(() => {
    console.error(error)
  }, [error])

  return (
    <EmptyState
      icon={CircleAlert}
      title="This page couldn't load"
      description="Something went wrong talking to the server. Please try again."
      action={<Button onClick={() => retry()}>Try again</Button>}
    />
  )
}
