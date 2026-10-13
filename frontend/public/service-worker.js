const CACHE = 'bakery-pos-shell-v1'
const CORE = ['/', '/manifest.webmanifest', '/icons/bakery-pos.svg']

self.addEventListener('install', (event) => {
  event.waitUntil((async () => {
    const cache = await caches.open(CACHE)
    await cache.addAll(CORE)
    const response = await fetch('/')
    const html = await response.text()
    const assets = [...html.matchAll(/(?:src|href)="(\/assets\/[^"]+)"/g)].map((match) => match[1])
    await cache.addAll([...new Set(assets)])
    await self.skipWaiting()
  })())
})

self.addEventListener('activate', (event) => {
  event.waitUntil((async () => {
    const keys = await caches.keys()
    await Promise.all(keys.filter((key) => key !== CACHE).map((key) => caches.delete(key)))
    await self.clients.claim()
  })())
})

self.addEventListener('fetch', (event) => {
  const request = event.request
  const url = new URL(request.url)
  if (request.method !== 'GET' || url.origin !== self.location.origin || url.pathname.startsWith('/api/')) return
  event.respondWith((async () => {
    const cached = await caches.match(request)
    if (cached) return cached
    try {
      const response = await fetch(request)
      if (response.ok) {
        const cache = await caches.open(CACHE)
        await cache.put(request, response.clone())
      }
      return response
    } catch (error) {
      if (request.mode === 'navigate') return (await caches.match('/')) ?? Response.error()
      throw error
    }
  })())
})
