// Actual Go/WASM browser resource/decoder/binding plus production C++ WASM and
// AudioWorklet processor, using Node transport shims and a simulated audio pump.
// This verifies wiring and source coordinates, never physical presentation.
import assert from 'node:assert/strict';
import { Worker } from 'node:worker_threads';
import { resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { createOutputQueue, OutputWriter, QueueState } from '../web/public/practice/output-queue.mjs';

let Processor;
globalThis.AudioWorkletProcessor = class {};
globalThis.registerProcessor = (_name, value) => { Processor = value; };
await import('../web/public/practice/output-worklet.mjs');
const worker = new Worker(new URL('./browser-practice-node-worker.mjs', import.meta.url), { workerData: {
 source: resolve('build/browser-conformance.wav'), name: 'fixture.wav', loader: resolve('web/public/practice/wasm_exec.js'), wasm: resolve('web/public/practice/core.wasm'), preview: true,
} });
let readyResolve, readyReject, next = 0;
const ready = new Promise((resolve, reject) => { readyResolve = resolve; readyReject = reject; });
const pending = new Map();
const timeout = setTimeout(() => { readyReject(new Error('Preview test timed out.')); for (const call of pending.values()) call.reject(new Error('Preview test timed out.')); worker.terminate(); }, 30000);
worker.on('error', error => { readyReject(error); for (const call of pending.values()) call.reject(error); });
worker.on('message', message => {
 if (message.type === 'ready') readyResolve();
 if (message.type === 'result') { const call = pending.get(message.id); pending.delete(message.id); call?.resolve(message); }
});
const call = (request, extra = {}) => new Promise((resolve, reject) => {
 const id = ++next; pending.set(id, { resolve, reject }); worker.postMessage({ id, request: { version: 1, ...request }, ...extra });
});
const requireResponse = message => { assert.equal(message.response.error, undefined); return message.response; };
let sequence = 0;
const command = async fields => { const response = requireResponse(await call({ type: 'command', command: { version: 1, sequence: sequence + 1, ...fields } })); sequence = response.snapshot.sequence; return response; };
async function preview(cancelAfter = 0) {
 const queue = createOutputQueue(1), writer = new OutputWriter(queue);
 const processor = new Processor({ processorOptions: { queue } });
 const channel = new Float32Array(128), graph = [[channel]], input = [];
 let finished = false, message, failed, cancelled = false, measured = 0, crossings = 0, previous = 0, copied = 0;
 const response = call({ type: 'preview' }, { queue });
 void response.then(value => { finished = true; message = value; }, error => { finished = true; failed = error; });
 while (!finished) {
  const before = writer.snapshot().copiedFrames;
  processor.process(input, graph);
  const after = writer.snapshot().copiedFrames;
  for (let i = 0; i < after - before; i++) {
   assert(Number.isFinite(channel[i]));
   if (copied + i > 4800 && copied + i < 27200) { if (previous <= 0 && channel[i] > 0) crossings++; measured++; }
   previous = channel[i];
  }
  copied = after;
  if (cancelAfter && copied >= cancelAfter && !cancelled) { worker.postMessage({ type: 'cancel' }); cancelled = true; }
  await delay(3);
 }
 if (failed) throw failed;
 if (cancelAfter) { assert.equal(message.response.error.code, 'cancelled'); assert.equal(writer.snapshot().state, QueueState.closed); }
 else {
  const report = requireResponse(message).preview;
  assert.equal(report.outputFrames, 32000); assert.equal(report.copiedFrames, 32000); assert.equal(report.sampleRate, 48000);
  assert.equal(writer.snapshot().state, QueueState.done);
  assert(Math.abs(crossings * 48000 / measured - 440 * 2 ** (3 / 12)) < 4);
 }
 assert(message.maxRead <= 65536);
 const snapshot = requireResponse(await call({ type: 'snapshot' })).snapshot;
 assert.equal(snapshot.sequence, sequence); assert.equal(snapshot.state, 'ready'); assert.equal(snapshot.capabilities.liveTransport, false);
 assert.equal(snapshot.project.settings.speed, .75); assert.equal(snapshot.project.settings.pitch, 3);
 return message.response;
}
try {
 await ready;
 const imported = requireResponse(await call({ type: 'import', source: 'source', document: 'project' }, { file: 'source' }));
 assert.equal(imported.snapshot.capabilities.streamPreview, true);
 assert.equal(imported.snapshot.capabilities.liveTransport, false);
 await command({ type: 'set_loop', loop: { enabled: true, start: .25, end: .75 } });
 await command({ type: 'set_parameters', speed: .75, pitch: 3 });
 const missingQueue = await call({ type: 'preview' }); assert.equal(missingQueue.response.error.code, 'host_failure');
 await preview();
 await preview(512);
 await preview(); // callbacks/cursors survive cancellation and release correctly
 const batch = requireResponse(await call({ type: 'events' })).events;
 assert.equal(batch.events.length, 2); assert(batch.events.every(event => event.type === 'session_changed'));
 console.log(JSON.stringify({ realGoWASMAndCPPWASM: true, sourceRange: [12000, 36000], outputFrames: 32000, pitch: 3, speed: .75, cancellationAndRetry: true, unchangedSessionSequence: sequence, noPlaybackCompletionEvents: true }, null, 2));
} finally { clearTimeout(timeout); await worker.terminate(); }
