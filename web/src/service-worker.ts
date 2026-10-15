import { appPath } from './app-url.ts';

export async function registerPracticeWorker(container = navigator.serviceWorker): Promise<ServiceWorkerRegistration> {
 return container.register(appPath('offline-sw.js'), {scope:appPath(''), updateViaCache:'none'});
}

// A forced network reload can have no controller even though an active worker
// and its caches already exist. Waiting for controllerchange then hangs: that
// worker won't activate again. A normal reload can immediately use it instead.
export async function prepareIsolatedReload(container = navigator.serviceWorker, timeoutMs = 30000): Promise<void> {
 const registration = await registerPracticeWorker(container);
 if (registration.active) {
  registration.active.postMessage({type:'claim'});
  return;
 }
 let timer: ReturnType<typeof setTimeout> | undefined;
 const worker = registration.installing ?? registration.waiting;
 let changed: (() => void) | undefined;
 try {
  await Promise.race([
   container.ready,
   new Promise<never>((_,reject) => {
    timer = setTimeout(() => reject(new Error('Preparation timed out. Reload while online to try again.')), timeoutMs);
    changed = () => { if (worker?.state === 'redundant') reject(new Error('Preparation failed. Reload while online to try again.')); };
    worker?.addEventListener('statechange',changed);
    changed();
   }),
  ]);
 } finally {
  clearTimeout(timer);
  if (changed) worker?.removeEventListener('statechange',changed);
 }
}
