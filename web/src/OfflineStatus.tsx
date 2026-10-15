import { useEffect, useState } from 'react';
import { appPath } from './app-url';

export default function OfflineStatus() {
 const [status, setStatus] = useState('');
 useEffect(() => {
  if (!import.meta.env.PROD) return;
  let active = true;
  const cleanups: (() => void)[] = [];
  const show = (message: string) => { if (active) setStatus(message); };
  if (!('serviceWorker' in navigator)) { show('Offline app storage is unavailable in this browser. Device projects can still be downloaded.'); return; }
  show('Preparing this app for offline practice…');
  void navigator.serviceWorker.register(appPath('offline-sw.js'), {scope:appPath(''), updateViaCache:'none'}).then(registration => {
   if (!active) return;
   const update = () => {
    if (registration.waiting) show('An app update is ready. Save your project, then close all Riffine tabs and reopen to use it.');
    else if (registration.active) show('App ready for offline practice. Reopen saved projects with their original audio.');
   };
   const watch = (worker: ServiceWorker | null) => {
    if (!worker) return;
    const changed = () => {
     update();
     if (worker.state === 'redundant' && !registration.active) show('Offline preparation failed. Keep this tab open and try reloading while online; download your project before closing.');
    };
    worker.addEventListener('statechange',changed); cleanups.push(() => worker.removeEventListener('statechange',changed));
   };
   const found = () => watch(registration.installing);
   registration.addEventListener('updatefound',found); cleanups.push(() => registration.removeEventListener('updatefound',found));
   watch(registration.installing); update();
   const controlled = () => update();
   navigator.serviceWorker.addEventListener('controllerchange',controlled); cleanups.push(() => navigator.serviceWorker.removeEventListener('controllerchange',controlled));
  }).catch(failure => show(`Offline app storage could not be prepared: ${failure instanceof Error ? failure.message : String(failure)}. Download your project before closing.`));
  return () => { active = false; cleanups.forEach(cleanup => cleanup()); };
 }, []);
 return status ? <p className="offline-status" role="status">{status}</p> : null;
}
