import { ImageResponse } from "next/og"

import { PwaIconArt } from "@/components/pwa-icon-art"

// iPhone home-screen icon (push notifications on iOS need the site added to
// the home screen).
export const size = { width: 180, height: 180 }
export const contentType = "image/png"

export default function AppleIcon() {
  return new ImageResponse(<PwaIconArt size={180} />, size)
}
