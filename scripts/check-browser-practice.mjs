import { Worker } from 'node:worker_threads';
import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import { deepStrictEqual, equal, ok } from 'node:assert/strict';
const root = resolve(import.meta.dirname, '..');
const fixture = JSON.parse(await readFile(`${root}/core/practice/testdata/session-v1.json`, 'utf8'));
const native = spawnSync(`${root}/build/host-spike/host-spike`, { input: JSON.stringify(fixture), encoding: 'utf8' });
equal(native.status, 0, native.stderr);
const expected = JSON.parse(native.stdout).binding;
ok(expected?.length, 'Native binding trace is absent');
const tone = spawnSync(`${root}/bin/riffine`, ['tone', '--out', `${root}/build/browser-conformance.wav`, '--seconds', String(fixture.audio.seconds), '--hz', String(fixture.audio.hz)], { encoding: 'utf8' });
if (tone.status !== 0 && !tone.stderr.includes('exist')) throw new Error(tone.stderr);
const worker = new Worker(new URL('./browser-practice-node-worker.mjs', import.meta.url), { workerData: { source: `${root}/build/browser-conformance.wav`, name: fixture.audio.name, wasm: `${root}/web/public/practice/core.wasm`, loader: `${root}/web/public/practice/wasm_exec.js` } });
let next = 0;
const pending = new Map();
let readyResolve, readyReject;
const ready = new Promise((resolve, reject) => { readyResolve = resolve; readyReject = reject; });
const timeout = setTimeout(() => { readyReject(new Error('Browser worker timed out')); for (const call of pending.values()) call.reject(new Error('Browser request timed out')); worker.terminate(); }, 30000);
worker.on('error', error => { readyReject(error); for (const call of pending.values()) call.reject(error); });
worker.on('message', data => {
 if (data.type === 'ready') readyResolve();
 if (data.type === 'result') { const call = pending.get(data.id); pending.delete(data.id); call?.resolve(data); }
});
const call = (request, file, document) => new Promise((resolve, reject) => { const id = ++next; pending.set(id, { resolve, reject }); worker.postMessage({ id, request: { version: 1, ...request }, file, document }); });
try {
 await ready;
 const requests = [{ type: 'import', source: 'source', document: 'project' }, ...fixture.steps.map(step => ({ type: 'command', command: step.command })), { type: 'waveform', startFrame: fixture.render.startFrame, endFrame: fixture.render.endFrame, bins: 7 }, { type: 'detail', startFrame: 255, endFrame: 258, bins: 3 }, { type: 'events' }, { type: 'export' }];
 let maxRead = 0, exported;
 for (let iteration = 0; iteration < 20; iteration++) {
  for (let j = 0; j < requests.length; j++) {
   const data = await call(requests[j], j === 0 ? 'source' : undefined);
   deepStrictEqual(data.response, expected[j]);
   maxRead = Math.max(maxRead, data.maxRead);
   if (requests[j].type === 'export') exported = data.response.document;
  }
 }
 const reopened = await call({ type: 'open', source: 'source', document: 'project' }, 'source', exported);
 equal(reopened.response.error, undefined);
 deepStrictEqual(reopened.response.snapshot.project, expected.at(-1).snapshot.project);
 const opening = call({ type: 'import', source: 'source', document: 'project' }, 'large');
 setTimeout(() => worker.postMessage({ type: 'cancel' }), 20);
 const cancelled = await opening;
 equal(cancelled.response.error?.code, 'cancelled');
 const retained = await call({ type: 'snapshot' });
 deepStrictEqual(retained.response.snapshot.project, reopened.response.snapshot.project);
 ok(maxRead <= 65536, `Unbounded File read: ${maxRead}`);
 console.log(JSON.stringify({ iterations: 20, responsesPerIteration: requests.length, projectReopen: true, cancelledLargeImportPreservesSession: true, maxFileReadBytes: maxRead }, null, 2));
} finally { clearTimeout(timeout); await worker.terminate(); }
