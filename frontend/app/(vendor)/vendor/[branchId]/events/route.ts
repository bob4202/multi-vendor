import { vendorAPI } from "@/lib/vendor/api"

// Relays the branch's live order stream from the API with the session
// token from the httpOnly cookie. Failure answers 204, which tells
// EventSource to stop; the page then falls back to polling.
export async function GET(
  request: Request,
  { params }: { params: Promise<{ branchId: string }> }
) {
  const { branchId } = await params
  const upstream = await vendorAPI
    .openBranchStream(branchId, request.signal)
    .catch(() => null)
  if (!upstream?.ok || !upstream.body) {
    return new Response(null, { status: 204 })
  }
  return new Response(upstream.body, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache, no-transform",
      "X-Accel-Buffering": "no",
    },
  })
}
