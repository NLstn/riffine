import { useEffect, useState } from 'react';
import { registerPracticeWorker } from './service-worker';

export default function OfflineStatus() {
 const [status, setStatus] = useState('');
 useEffect(() => {
  if (!import.meta.env.PROD) return;
  let active = true;
  const cleanups: (() => void)[] = [];
  const show = (message: string) => { if (active) setStatus(message); };
  if (!('serviceWorker' in navigator)) { show('Offline app storage is unavailable in this browser. Device projects can still be downloaded.'); return; }
  show('Preparing this app for offline practice…');
  void registerPracticeWorker().then(async registration => {
   if (!active) return;
   const update = () => {
    if (registration.waiting) show('An app update is ready. Save your project, then close all Riffine tabs and reopen to use it.');
   };
   const watch = (worker: ServiceWorker | null) => {
    if (!worker) return;
    worker.addEventListener('statechange',update); cleanups.push(() => worker.removeEventListener('statechange',update));
   };
   const found = () => watch(registration.installing);
   registration.addEventListener('updatefound',found); cleanups.push(() => registration.removeEventListener('updatefound',found));
   watch(registration.installing); update();
   const ready = await navigator.serviceWorker.ready;
   if (!active) return;
   const worker = navigator.serviceWorker.controller ?? ready.active;
   if (!worker) throw new Error('Offline worker is unavailable.');
   const channel = new MessageChannel();
   cleanups.push(() => channel.port1.close());
   channel.port1.onmessage = event => {
    if (registration.waiting) { update(); return; }
    if (event.data?.state === 'ready') show('App ready for offline practice. Reopen saved projects with their original audio.');
    if (event.data?.state === 'failed') show('Offline preparation was interrupted. You can keep practicing online; reload to retry, or download your project before closing.');
   };
   worker.postMessage({type:'prepare_offline'}, [channel.port2]);
  }).catch(failure => show(`Offline app storage could not be prepared: ${failure instanceof Error ? failure.message : String(failure)}. Download your project before closing.`));
  return () => { active = false; cleanups.forEach(cleanup => cleanup()); };
 }, []);
 return status ? <p className="offline-status" role="status">{status}</p> : null;
}
