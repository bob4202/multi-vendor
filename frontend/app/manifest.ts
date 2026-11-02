import type { MetadataRoute } from "next"

// Lets customers add the shop to their home screen - which iPhones require
// before they'll show push notifications.
export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "Grocery Shop",
    short_name: "Grocery Shop",
    description: "Order fresh groceries for collection or delivery.",
    start_url: "/",
    display: "standalone",
    background_color: "#ffffff",
    theme_color: "#171717",
    icons: [
      { src: "/pwa-icon/192", sizes: "192x192", type: "image/png" },
      { src: "/pwa-icon/512", sizes: "512x512", type: "image/png" },
      {
        src: "/pwa-icon/512",
        sizes: "512x512",
        type: "image/png",
        purpose: "maskable",
      },
    ],
  }
}
