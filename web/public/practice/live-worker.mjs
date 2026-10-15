import { ready } from './preview-worker.mjs';
import { ScheduledSelections } from './scheduled-selections.mjs';
let next = 0;
const pending = new Map();
function hostCall(method, args = {}) {
 const id = ++next;
 if (!Number.isSafeInteger(id) || pending.size >= 8) return Promise.reject(new Error('Live host control queue exhausted.'));
 return new Promise((resolve, reject) => { pending.set(id, {resolve, reject}); self.postMessage({type:'live_call', id, method, args}); });
}
export function receive(message) {
 if (message.type !== 'live_result') return false;
 const call = pending.get(message.id); pending.delete(message.id);
 if (message.error) call?.reject(new Error(message.error)); else call?.resolve(message.result);
 return true;
}
class Device {
 constructor(module, info, readPCM, readClicks) {
  this.cuesSupported=typeof module._riffine_click_mix_window==='function';this.metronomeSupported=this.cuesSupported&&typeof readClicks==='function';
  this.closed = false; this.closing = undefined; this.info = info;
  this.processing = new ScheduledSelections(module, info, (start, count) => new Promise((resolve, reject) => readPCM(start, count, resolve, reject)), error => {
   void hostCall('fail', {message:error.message}).catch(() => {});
  },readClicks ? (schedule,start,count)=>new Promise((resolve,reject)=>readClicks(schedule.generation,schedule.sequence,start,count,resolve,reject)) : undefined);
 }
 async operate(method, args) {
  if (method === 'close') return this.close();
  if (this.closed) throw new Error('Browser live device has closed.');
  if (method === 'reset') {
   await hostCall('suspend');
   await this.processing.stopProcessing();
   const buffers = await hostCall('reset', this.info);
   if (this.closed) throw new Error('Browser live device has closed.');
   await this.processing.reset(args.generation, buffers.queue, buffers.presented);
   return {};
  }
  if (method === 'schedule') { this.processing.schedule(args); return {}; }
  if (method === 'configure') return this.processing.configure(args.sequence,args.speed,args.pitch);
  if (method === 'start') { await this.processing.prefilled(); await hostCall('start'); return {}; }
  if (method === 'suspend') { await hostCall('suspend'); return this.processing.drain(128); }
  throw new Error('Unknown browser live control operation.');
 }
 // Every failed asynchronous control operation fences output before its callback.
 // Reset remains available on the borrowed source after the UI rearms a context.
 call(method, data, complete, failed) {
  const args = JSON.parse(data);
  let cancelled = false;
  void this.operate(method, args).then(result => { if (cancelled) throw new Error('Live control cancelled.'); complete(JSON.stringify(result)); }).catch(async error => {
   await this.processing.stopProcessing();
   try { await hostCall('fail', {message:String(error)}); } catch {}
   failed(String(error));
  });
  return fence => { cancelled = true; void this.close().then(() => fence(''), error => fence(String(error))); };
 }
 drain(limit) { try { return {data:JSON.stringify(this.processing.drain(limit))}; } catch (error) { return {error:String(error)}; } }
 async close() {
  if (!this.closing) {
   this.closed = true;
   this.closing = (async () => { await this.processing.close(); await hostCall('close'); })();
  }
  await this.closing;
 }
}
export function open(info, readPCM, readClicks, complete, failed) {
 let device, cancelled = false;
 const opening = (async () => {
  try {
   const module = await ready;
   if (cancelled) throw new Error('Live output opening cancelled.');
   await hostCall('open', info);
   device = new Device(module, info, readPCM,readClicks);
   if (cancelled) { await device.close(); throw new Error('Live output opening cancelled.'); }
   complete(device);
  } catch (error) {
   if (device) await device.close().catch(() => {});
   else await hostCall('close').catch(() => {});
   failed(String(error));
  }
 })();
 return fence => { cancelled = true; void opening.then(async () => { if (device) await device.close(); }).then(() => fence(''), error => fence(String(error))); };
}
