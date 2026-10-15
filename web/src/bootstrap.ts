import { appPath } from './app-url';

// Static hosts cannot set the isolation headers needed by live audio. Our
// service worker supplies them, then one reload starts an isolated document.
async function start() {
 if (import.meta.env.PROD && !globalThis.crossOriginIsolated) {
  const root = document.getElementById('root')!;
  root.textContent = 'Preparing Riffine for practice…';
  const attemptKey = `riffine-isolation:${appPath('')}`;
  try {
   if (!('serviceWorker' in navigator)) throw new Error('This browser cannot prepare live audio. Please use a browser with service worker support.');
   if (sessionStorage.getItem(attemptKey)) throw new Error('Live audio could not start. Close this tab and reopen Riffine, or enable service workers in your browser.');
   const controlled = new Promise<void>((resolve, reject) => {
    const timer = setTimeout(() => { cleanup(); reject(new Error('Preparation timed out. Reload while online to try again.')); }, 90000);
    const changed = () => { if (navigator.serviceWorker.controller) { cleanup(); resolve(); } };
    const cleanup = () => { clearTimeout(timer); navigator.serviceWorker.removeEventListener('controllerchange', changed); };
    navigator.serviceWorker.addEventListener('controllerchange', changed);
    changed();
   });
   // Observe rejection even if registration fails before control is acquired.
   void controlled.catch(() => {});
   await navigator.serviceWorker.register(appPath('offline-sw.js'), {scope:appPath(''), updateViaCache:'none'});
   await controlled;
   sessionStorage.setItem(attemptKey, '1');
   location.reload();
  } catch (error) {
   root.textContent = error instanceof Error ? error.message : String(error);
  }
  return;
 }
 if (import.meta.env.PROD) sessionStorage.removeItem(`riffine-isolation:${appPath('')}`);
 await import('./main');
}

void start();
