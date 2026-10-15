/* global VERSION, BASE, ASSETS */
const PREFIX = `riffine-shell-${encodeURIComponent(BASE)}-`;
const CACHE = `${PREFIX}${VERSION}`;
const required = new Set(ASSETS);
function isolated(response) {
 const headers = new Headers(response.headers);
 headers.set('Cross-Origin-Opener-Policy', 'same-origin');
 headers.set('Cross-Origin-Embedder-Policy', 'require-corp');
 return new Response(response.body, {status:response.status, statusText:response.statusText, headers});
}
self.addEventListener('install', event => {
 event.waitUntil((async () => {
  const cache = await caches.open(CACHE);
  try {
   // Fetch every exact asset before accepting this version. A failed install
   // leaves the old complete cache active rather than a partial update.
   for (const path of ASSETS) {
    const response = await fetch(new Request(path, {cache:'reload'}));
    if (!response.ok || (path !== BASE+'index.html' && response.headers.get('Content-Type')?.includes('text/html'))) throw new Error(`Offline asset unavailable: ${path}`);
    await cache.put(path,response);
   }
  } catch (error) { await caches.delete(CACHE); throw error; }
 })());
});
self.addEventListener('activate', event => {
 event.waitUntil((async () => {
  for (const name of await caches.keys()) if (name.startsWith(PREFIX) && name !== CACHE) await caches.delete(name);
  await self.clients.claim();
 })());
});
self.addEventListener('fetch', event => {
 const request = event.request, url = new URL(request.url);
 if (request.method !== 'GET' || url.origin !== self.location.origin || !url.pathname.startsWith(BASE)) return;
 const path = request.mode === 'navigate' ? BASE+'index.html' : url.pathname;
 if (!required.has(path)) return;
 event.respondWith((async () => {
  const cache = await caches.open(CACHE);
  const stored = await cache.match(path);
  if (stored) return isolated(stored);
  // Browser storage can be evicted. Surface network failure instead of
  // synthesizing a success or returning HTML for a missing worker/WASM file.
  return isolated(await fetch(request));
 })());
});
// Do not force an update into an active session. The next version activates
// after all tabs using the old version have closed.
self.addEventListener('message', event => {
 if (event.data?.type === 'offline_status') event.ports?.[0]?.postMessage({version:VERSION});
});
