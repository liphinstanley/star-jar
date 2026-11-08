// Service worker: makes star-jar installable and shows a friendly page when offline.
// Bump VERSION when files in PRECACHE change, so phones drop the old copies.
const VERSION = "v7";
const CACHE = `star-jar-${VERSION}`;
const PRECACHE = [
  "/static/offline.html",
  "/static/style.css",
  "/static/htmx.min.js",
  "/static/app.js",
  "/static/icon-192.png",
];

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(CACHE).then((c) => c.addAll(PRECACHE)));
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))
    )
  );
  self.clients.claim();
});

// Network first: stars must always be fresh, so the cache is only a fallback.
self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return; // never touch taps and approvals

  event.respondWith(
    fetch(req).catch(async () => {
      const cached = await caches.match(req);
      if (cached) return cached;
      if (req.mode === "navigate") return caches.match("/static/offline.html");
      return Response.error();
    })
  );
});
