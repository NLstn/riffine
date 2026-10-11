// Controlled FileReaderSync substitute exercises the real Go browser adapter.
// Actual browser FileReaderSync is verified separately through the workspace.
import { parentPort, workerData } from 'node:worker_threads';
import { readFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
const bytes = new Uint8Array(await readFile(workerData.source));
let reads = 0, maxRead = 0;
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
globalThis.postMessage = data => parentPort.postMessage({ ...data, reads, maxRead });
await import(pathToFileURL(workerData.loader));
if (workerData.preview) {
 globalThis.self = globalThis;
 const preview = await import('../web/public/practice/preview-worker.mjs');
 await preview.ready;
 globalThis.riffinePreviewRun = preview.run;
 globalThis.riffinePreviewEnabled = true;
}
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(await readFile(workerData.wasm), go.importObject);
void go.run(instance).catch(error => { throw error; });
parentPort.on('message', data => {
 if (data.request?.type === 'preview') globalThis.riffinePreviewQueue = data.queue;
 if (data.type === 'cancel') { globalThis.riffinePracticeCancel(); return; }
 let file;
 if (data.file === 'source') file = new LocalFile(bytes);
 if (data.file === 'large') {
  const header = bytes.slice(0, 44);
  const size = 32 * 1024 * 1024;
  new DataView(header.buffer).setUint32(4, size + 36, true);
  new DataView(header.buffer).setUint32(40, size, true);
  file = new LocalFile(header, size + 44);
 }
 globalThis.riffinePracticeRequest(data.id, JSON.stringify(data.request), file, data.document ?? '');
});
