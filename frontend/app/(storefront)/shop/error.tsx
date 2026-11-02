"use client"

import { useEffect } from "react"
import { CircleAlert } from "lucide-react"

import { EmptyState } from "@/components/common/empty-state"
import { Container } from "@/components/layout/container"
import { Button } from "@/components/ui/button"

export default function ShopError({
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
    <Container className="py-8">
      <EmptyState
        icon={CircleAlert}
        title="We couldn't load the shop"
        description="Something went wrong on our end. Please try again."
        action={<Button onClick={() => retry()}>Try again</Button>}
      />
    </Container>
  )
}
