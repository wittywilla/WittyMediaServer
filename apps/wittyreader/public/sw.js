// WittyReader's service worker. It serves the unpacked files of the open
// EPUB (stored in Cache Storage with a strict Content-Security-Policy when
// the book is opened), and keeps the app itself working offline after the
// first visit. Nothing here talks to any other site.
const APP = 'wittyreader-app-v1'
const BOOK = 'wittyreader-book'

self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', (e) => e.waitUntil((async () => {
  for (const k of await caches.keys()) if (k.startsWith('wittyreader-app-') && k !== APP) await caches.delete(k)
  await self.clients.claim()
})()))

self.addEventListener('fetch', (e) => {
  const req = e.request
  if (req.method !== 'GET') return
  const url = new URL(req.url)
  if (url.origin !== self.location.origin) return
  const scope = new URL(self.registration.scope)
  if (url.pathname.startsWith(scope.pathname + 'book/')) {
    e.respondWith(caches.open(BOOK)
      .then((c) => c.match(url.origin + url.pathname))
      .then((r) => r || new Response('Not in this book', { status: 404, headers: { 'Content-Type': 'text/plain' } })))
    return
  }
  // The app's own files: the network first, the copy from the last visit when offline.
  e.respondWith(fetch(req).then((res) => {
    if (res.ok && res.type === 'basic') { const copy = res.clone(); caches.open(APP).then((c) => c.put(req, copy)) }
    return res
  }).catch(() => caches.match(req).then((r) => r || caches.match(scope.href)).then((r) => r || Response.error())))
})
