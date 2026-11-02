// Service worker: shows order updates pushed by the API, and opens the
// order page when one is tapped. Payload: { title, body, url, tag }.

self.addEventListener("push", (event) => {
  let data = {}
  try {
    data = event.data ? event.data.json() : {}
  } catch {
    data = { body: event.data ? event.data.text() : "" }
  }
  event.waitUntil(
    self.registration.showNotification(data.title || "Order update", {
      body: data.body || "",
      // A newer update for the same order replaces the old one.
      tag: data.tag,
      renotify: Boolean(data.tag),
      icon: "/pwa-icon/192",
      badge: "/pwa-icon/192",
      data: { url: data.url || "/" },
    })
  )
})

self.addEventListener("notificationclick", (event) => {
  event.notification.close()
  const url = new URL(event.notification.data?.url || "/", self.location.origin)
    .href
  event.waitUntil(
    self.clients
      .matchAll({ type: "window", includeUncontrolled: true })
      .then((windows) => {
        const open = windows.find((w) => w.url === url)
        return open ? open.focus() : self.clients.openWindow(url)
      })
  )
})
