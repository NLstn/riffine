/* global VERSION, BASE, ASSETS, SHELL, HASHES */
const PREFIX = `riffine-shell-${encodeURIComponent(BASE)}-`;
const CACHE = `${PREFIX}${VERSION}`;
const COMPLETE = BASE+'__offline_complete__';
const required = new Set(ASSETS);
const pending = new Map();
let preparation;
function isolated(response) {
 const headers = new Headers(response.headers);
 headers.set('Cross-Origin-Opener-Policy', 'same-origin');
 headers.set('Cross-Origin-Embedder-Policy', 'require-corp');
 return new Response(response.body, {status:response.status, statusText:response.statusText, headers});
}
async function loadAsset(path) {
 const cache = await caches.open(CACHE);
 const stored = await cache.match(path);
 if (stored) return stored;
 // Reuse only bytes identified by the same build hash. Versioned query strings
 // also prevent a browser/CDN's old response for a stable filename being reused.
 for (const name of await caches.keys()) {
  if (!name.startsWith(PREFIX) || name === CACHE) continue;
  const previous = await (await caches.open(name)).match(path);
  if (previous?.headers.get('X-Riffine-Asset-Hash') === HASHES[path]) {
   await cache.put(path,previous.clone());
   return previous;
  }
 }
 const response = await fetch(new Request(`${path}?v=${HASHES[path]}`, {cache:'force-cache'}));
 if (!response.ok || (path !== BASE+'index.html' && response.headers.get('Content-Type')?.includes('text/html'))) throw new Error(`Offline asset unavailable: ${path}`);
 const headers = new Headers(response.headers);
 headers.set('X-Riffine-Asset-Hash',HASHES[path]);
 const asset = new Response(response.body, {status:response.status, statusText:response.statusText, headers});
 await cache.put(path,asset.clone());
 return asset;
}
function asset(path) {
 if (!pending.has(path)) {
  const request = loadAsset(path).finally(() => pending.delete(path));
  pending.set(path,request);
 }
 return pending.get(path).then(response => response.clone());
}
async function download(paths, progress = () => {}) {
 let next = 0, completed = 0, failure;
 // Stop assigning work after a failure and wait for in-flight writes before
 // reporting it. Installation can then safely discard its partial shell.
 await Promise.all(Array.from({length:Math.min(6,paths.length)}, async () => {
  while (!failure && next < paths.length) {
   const path = paths[next++];
   try { await asset(path); progress(++completed,paths.length); }
   catch (error) { failure ??= error; }
  }
 }));
 if (failure) throw failure;
}
async function prepareOffline() {
 const cache = await caches.open(CACHE);
 if (await cache.match(COMPLETE)) return;
 await download(ASSETS);
 await cache.put(COMPLETE,new Response(VERSION));
 // Retain the last complete package until its replacement is complete too.
 for (const name of await caches.keys()) if (name.startsWith(PREFIX) && name !== CACHE) await caches.delete(name);
}
self.addEventListener('install', event => {
 event.waitUntil((async () => {
  try { await download(SHELL); }
  catch (error) { await caches.delete(CACHE); throw error; }
 })());
});
self.addEventListener('activate', event => {
 event.waitUntil(self.clients.claim());
});
self.addEventListener('fetch', event => {
 const request = event.request, url = new URL(request.url);
 if (request.method !== 'GET' || url.origin !== self.location.origin || !url.pathname.startsWith(BASE)) return;
 const path = url.pathname === BASE ? BASE+'index.html' : url.pathname;
 if (!required.has(path)) return;
 // Cache misses (including after storage eviction) are written through, so a
 // subsequent reload does not repeatedly download the same missing resources.
 event.respondWith(asset(path).then(isolated));
});
// Never force an update into an active practice session. A replacement worker
// waits until the previous version's tabs close, using the browser lifecycle.
self.addEventListener('message', event => {
 if (event.data?.type === 'claim') { event.waitUntil(self.clients.claim()); return; }
 if (event.data?.type === 'offline_status') {
  event.waitUntil((async () => {
   const complete = !!(await (await caches.open(CACHE)).match(COMPLETE));
   event.ports?.[0]?.postMessage({version:VERSION,complete});
  })());
 }
 if (event.data?.type === 'prepare_offline') {
  const reply = message => event.ports?.[0]?.postMessage(message);
  event.waitUntil((async () => {
   try {
    reply({state:'preparing'});
    preparation ??= prepareOffline().finally(() => { preparation = undefined; });
    await preparation;
    reply({state:'ready',version:VERSION});
   } catch (error) { reply({state:'failed',message:String(error)}); }
  })());
 }
});
