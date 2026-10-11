// Node worker transport shim only: loads the production browser processing worker
// and its actual Emscripten module, without replacing any processing algorithm.
import { parentPort } from 'node:worker_threads';
globalThis.self = {
 postMessage: (value, transfer) => parentPort.postMessage(value, transfer),
 onmessage: undefined,
};
await import('../web/public/practice/processing-worker.mjs');
parentPort.on('message', data => self.onmessage({ data }));
