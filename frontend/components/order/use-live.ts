"use client"

import { useEffect, useRef } from "react"

// Listens to a Server-Sent Events stream and calls onEvent for each event of
// `eventName`. If the stream can't be opened (or the server says stop), it
// falls back to calling `poll` every pollMs instead. Inactive = do nothing.
export function useLive<T>({
  url,
  eventName,
  onEvent,
  poll,
  pollMs,
  active = true,
}: {
  url: string
  eventName: string
  onEvent: (data: T) => void
  poll?: () => void
  pollMs: number
  active?: boolean
}) {
  // Latest callbacks without reconnecting when they change.
  const onEventRef = useRef(onEvent)
  const pollRef = useRef(poll)
  useEffect(() => {
    onEventRef.current = onEvent
    pollRef.current = poll
  })

  useEffect(() => {
    if (!active) return
    let timer: number | undefined
    const startPolling = () => {
      if (timer === undefined && pollRef.current) {
        timer = window.setInterval(() => pollRef.current?.(), pollMs)
      }
    }

    if (typeof EventSource === "undefined") {
      startPolling()
      return () => window.clearInterval(timer)
    }

    const source = new EventSource(url)
    source.addEventListener(eventName, (e) => {
      try {
        onEventRef.current(JSON.parse((e as MessageEvent).data) as T)
      } catch {
        // a malformed event is skipped; the next one catches up
      }
    })
    source.onerror = () => {
      // CONNECTING = the browser is retrying by itself; CLOSED = it gave up.
      if (source.readyState === EventSource.CLOSED) startPolling()
    }
    return () => {
      source.close()
      window.clearInterval(timer)
    }
  }, [url, eventName, pollMs, active])
}
