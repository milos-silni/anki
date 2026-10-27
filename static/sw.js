// Minimal service worker so the app can be installed. Everything goes to the
// network; when offline, pages get a short notice instead of Chrome's error.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => e.waitUntil(self.clients.claim()));

self.addEventListener("fetch", (e) => {
  if (e.request.mode !== "navigate") return;
  e.respondWith(
    fetch(e.request).catch(() => new Response(
      '<!doctype html><meta name="viewport" content="width=device-width"><title>Offline</title>' +
      '<body style="font:18px system-ui;text-align:center;padding:3rem">Offline – connect to the internet to study.</body>',
      { headers: { "Content-Type": "text/html; charset=utf-8" } }))
  );
});
