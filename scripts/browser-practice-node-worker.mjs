// Controlled FileReaderSync substitute exercises the real Go browser adapter.
// Actual browser FileReaderSync is verified separately through the workspace.
import { parentPort, workerData } from 'node:worker_threads';
import { readFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
const bytes = new Uint8Array(await readFile(workerData.source));
let reads = 0, maxRead = 0, codecOpens = 0, knownInfoOpens = 0;
class LocalFile {
 constructor(data, size = data.length) { this.bytes = data; this.size = size; this.name = workerData.name; }
 slice(start, end) { return { file: this, start, end: Math.min(end, this.size) }; }
}
globalThis.FileReaderSync = class {
 readAsArrayBuffer({ file, start, end }) {
  reads++; maxRead = Math.max(maxRead, end - start);
  const result = new Uint8Array(end - start);
  if (start < file.bytes.length) result.set(file.bytes.subarray(start, Math.min(end, file.bytes.length)));
  return result.buffer;
 }
};
globalThis.postMessage = data => parentPort.postMessage({ ...data, reads, maxRead, codecOpens, knownInfoOpens });
await import(pathToFileURL(workerData.loader));
let liveReceive, projectStore, analysisCache;
if (workerData.storage || workerData.analysisCache) {
 const {createRequire} = await import('node:module');
 const require = createRequire(new URL('../web/package.json',import.meta.url));
 globalThis.indexedDB = new (require('fake-indexeddb').IDBFactory)();
 const store = await import('../web/public/practice/project-store.mjs');
 projectStore = store.projectStore;
 globalThis.riffineProjectStoreRun = store.runProjectStore;
 if (workerData.analysisCache) {
  analysisCache = store.analysisCache;
  globalThis.riffineAnalysisCacheRun = store.runAnalysisCache;
  globalThis.riffinePeakCacheRun = store.runPeakCache;
 }
}
if (workerData.codecs || workerData.preview || workerData.live) {
 globalThis.self = globalThis;
 const codec = await import('../web/public/practice/codec-port.mjs');
 globalThis.riffineCodecOpen = (...args) => {codecOpens++;if(args[4])knownInfoOpens++;return codec.openCodec(...args);};
}
if (workerData.preview || workerData.live) {
 const preview = await import('../web/public/practice/preview-worker.mjs');
 await preview.ready;
 globalThis.riffinePreviewRun = preview.run;
 globalThis.riffinePreviewEnabled = true;
 if (workerData.live) { const live = await import('../web/public/practice/live-worker.mjs'); liveReceive = live.receive; globalThis.riffineLiveOpen = live.open; globalThis.riffineLiveEnabled = true; }
}
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(await readFile(workerData.wasm), go.importObject);
void go.run(instance).catch(error => { throw error; });
parentPort.on('message', data => {
 if (data.type === 'host_analysis') { void analysisCache[data.method](...data.args).then(result => parentPort.postMessage({type:'analysis_result',id:data.id,result}), error => parentPort.postMessage({type:'analysis_result',id:data.id,error:error.message})); return; }
 if (data.type === 'host_store') { void projectStore[data.method](...data.args).then(result => parentPort.postMessage({type:'storage_result',id:data.id,result}), error => parentPort.postMessage({type:'storage_result',id:data.id,error:error.message})); return; }
 if (liveReceive?.(data)) return;
 if (data.request?.type === 'preview') globalThis.riffinePreviewQueue = data.queue;
 if (data.type === 'cancel') { globalThis.riffinePracticeCancel(); return; }
 let file;
 if (data.file === 'source') file = new LocalFile(bytes);
 if (data.file === 'changed') {const changed = bytes.slice(); changed[changed.length-1] ^= 1; file = new LocalFile(changed);}
 if (data.file === 'large') {
  const header = bytes.slice(0, 44);
  const size = 32 * 1024 * 1024;
  new DataView(header.buffer).setUint32(4, size + 36, true);
  new DataView(header.buffer).setUint32(40, size, true);
  file = new LocalFile(header, size + 44);
 }
 globalThis.riffinePracticeRequest(data.id, JSON.stringify(data.request), file, data.document ?? '');
});
