import { externalAPI } from "@/lib/external-api-service"

// Relays the order's live stream (Server-Sent Events) from the API, adding
// the storefront key on the way, so the browser never holds it.
//
// Any failure answers 204: EventSource treats that as "stop reconnecting",
// and the page falls back to polling.
export async function GET(
  request: Request,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id } = await params
  const upstream = await externalAPI
    .openOrderStream(id, request.signal)
    .catch(() => null)
  if (!upstream?.ok || !upstream.body) {
    return new Response(null, { status: 204 })
  }
  return new Response(upstream.body, {
    headers: {
      "Content-Type": "text/event-stream",
      // no-transform keeps compression from buffering the stream.
      "Cache-Control": "no-cache, no-transform",
      "X-Accel-Buffering": "no",
    },
  })
}
