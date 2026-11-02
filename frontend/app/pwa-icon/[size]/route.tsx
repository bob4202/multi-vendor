import { ImageResponse } from "next/og"

import { PwaIconArt } from "@/components/pwa-icon-art"

// PNG icons for the web app manifest (/pwa-icon/192, /pwa-icon/512).
const SIZES = new Set([192, 512])

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ size: string }> }
) {
  const size = Number((await params).size)
  if (!SIZES.has(size)) return new Response("Not found", { status: 404 })
  return new ImageResponse(<PwaIconArt size={size} />, {
    width: size,
    height: size,
    headers: { "Cache-Control": "public, max-age=86400, immutable" },
  })
}
