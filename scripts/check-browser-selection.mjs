import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { Worker } from 'node:worker_threads';
import { performance } from 'node:perf_hooks';

const nativePath = process.argv[2] ?? (process.platform === 'win32' ? 'build/native/Release/riffine-selection-test.exe' : 'build/native/riffine-selection-test');
const expected = JSON.parse(execFileSync(nativePath, { encoding: 'utf8' }));
const worker = new Worker(new URL('./browser-selection-node-worker.mjs', import.meta.url));
let next = 0;
const pending = new Map();
let readyResolve, readyReject;
const ready = new Promise((resolve, reject) => { readyResolve = resolve; readyReject = reject; });
const timeout = setTimeout(() => { readyReject(new Error('Processing worker startup timed out.')); worker.terminate(); }, 30000);
worker.on('error', error => { readyReject(error); for (const call of pending.values()) call.reject(error); });
worker.on('message', data => {
 if (data.type === 'ready') { clearTimeout(timeout); readyResolve(data); }
 if (data.type === 'fatal') readyReject(new Error(data.message));
 if (data.type === 'result') {
  const call = pending.get(data.id); pending.delete(data.id);
  if (data.error) call?.reject(new Error(data.error)); else call?.resolve(data.result);
 }
});
const call = (type, fields = {}, transfers = []) => new Promise((resolve, reject) => {
 const id = ++next; pending.set(id, { resolve, reject });
 worker.postMessage({ id, type, ...fields }, transfers);
});
async function run(speed, pitch) {
 const initial = await call('create', { channels: 2, sampleRate: 48000, sourceFrames: 48000, speed, pitch });
 assert.equal(initial.outputFrames, Math.round(48000 / speed));
 await assert.rejects(call('read', { capacity: 0 }));
 await assert.rejects(call('push', { pcm: new Float32Array(1026) }));
 let read = 0, written = 0, measured = 0;
 const crossings = [0, 0], energy = [0, 0], previous = [0, 0];
 for (;;) {
  const { pcm, frames, needsInput } = await call('read', { capacity: 113 });
  assert(pcm instanceof Float32Array);
  assert.equal(pcm.length, frames * 2);
  assert(frames <= 113);
  for (let i = 0; i < frames; i++) {
   const measure = written + i > 4800 && written + i + 4800 < initial.outputFrames;
   for (let c = 0; c < 2; c++) {
    const value = pcm[i * 2 + c]; assert(Number.isFinite(value));
    if (measure) { if (previous[c] <= 0 && value > 0) crossings[c]++; energy[c] += value * value; }
    if (written + i === 0 || written + i === initial.outputFrames - 1) assert(value === 0);
    previous[c] = value;
   }
   if (measure) measured++;
  }
  written += frames;
  if (needsInput) {
   const frames = Math.min(512, 48000 - read);
   assert(frames > 0);
   const pcm = new Float32Array(frames * 2);
   for (let i = 0; i < frames; i++) for (let c = 0; c < 2; c++) pcm[i * 2 + c] = .2 * Math.sin(2 * Math.PI * (c ? 660 : 440) * (read + i) / 48000);
   await call('push', { pcm }, [pcm.buffer]); read += frames;
  } else if (!frames) break;
 }
 const info = await call('info'); assert.equal(info.consumedSourceFrames, 48000);
 assert.equal(written, initial.outputFrames);
 await assert.rejects(call('push', { pcm: new Float32Array(2) }));
 const eof = await call('read', { capacity: 1 }); assert.equal(eof.frames, 0); assert.equal(eof.needsInput, false);
 await call('close'); await call('close');
 return { speed, pitch, outputFrames: written, frequency: crossings.map(n => n * 48000 / measured), rms: energy.map(n => Math.sqrt(n / measured)) };
}
async function faded(frames, sampleRate, edgeFadeFrames) {
 await call('create', {channels: 1, sampleRate, sourceFrames: frames, speed: 1, pitch: 0, edgeFadeFrames});
 const result = new Float32Array(frames); let read = 0, written = 0;
 for (;;) {
  const block = await call('read', {capacity: 113});
  result.set(block.pcm, written); written += block.frames;
  if (block.needsInput) {
   const count = Math.min(512, frames - read), pcm = new Float32Array(count);
   for (let i = 0; i < count; i++) pcm[i] = .2 * Math.cos(2 * Math.PI * 440 * (read + i) / sampleRate);
   await call('push', {pcm}, [pcm.buffer]); read += count;
  } else if (!block.frames) break;
 }
 assert.equal(read, frames); assert.equal(written, frames); await call('close'); return result;
}
try {
 const initial = await ready;
 assert.equal(initial.heapBytes, 33554432);
 await assert.rejects(call('create', { channels: 2, sampleRate: 48000, sourceFrames: 0, speed: 1, pitch: 0 }));
 for (const sampleRate of [8000, 48000, 192000]) {
  await call('create', { channels: 1, sampleRate, sourceFrames: 1, speed: 2, pitch: 0 });
  await assert.rejects(call('push', { pcm: new Float32Array([Infinity]) }));
  assert.equal((await call('info')).consumedSourceFrames, 0);
  await call('push', { pcm: new Float32Array([.2]) });
  const frame = await call('read', { capacity: 1 }); assert.equal(frame.frames, 1); assert.equal(frame.needsInput, false);
  const eof = await call('read', { capacity: 1 }); assert.equal(eof.frames, 0);
  await call('close');
 }
 // A huge coordinate range must not allocate storage proportional to duration.
 const huge = await call('create', { channels: 2, sampleRate: 48000, sourceFrames: 2 ** 40, speed: .25, pitch: 0 });
 assert.equal(huge.outputFrames, 2 ** 42); await call('close');
 for (const sampleRate of [8000, 44100, 48000, 192000]) for (const frames of [1, 2, 3, 4095]) {
  const baseline = await faded(frames, sampleRate, 0);
  const fade = Math.min(Math.floor(sampleRate / 200), Math.ceil(frames / 2));
  const scheduled = await faded(frames, sampleRate, fade);
  const defaults = await faded(frames, sampleRate, undefined);
  const defaultFade = Math.min(Math.round(sampleRate * .005), Math.floor(frames / 2));
  for (let i = 0; i < frames; i++) {
   const gain = n => n ? Math.min(1, i / n, (frames - 1 - i) / n) : 1;
   assert(Math.abs(scheduled[i] - baseline[i] * gain(fade)) < 2e-7);
   assert(Math.abs(defaults[i] - baseline[i] * gain(defaultFade)) < 2e-7);
  }
 }
 for (const edgeFadeFrames of [-1, .5, NaN, Infinity, 2049]) await assert.rejects(call('create', {channels: 1, sampleRate: 44100, sourceFrames: 4095, speed: 1, pitch: 0, edgeFadeFrames}));
 const started = performance.now();
 let maxRMSDifference = 0;
 for (let iteration = 0; iteration < 20; iteration++) {
  for (const reference of expected.cases) {
   const actual = await run(reference.speed, reference.pitch);
   assert.equal(actual.outputFrames, reference.outputFrames);
   for (let c = 0; c < 2; c++) {
    assert(Math.abs(actual.frequency[c] - reference.frequency[c]) < 4);
    maxRMSDifference = Math.max(maxRMSDifference, Math.abs(actual.rms[c] - reference.rms[c]));
    assert(Math.abs(actual.rms[c] - reference.rms[c]) <= .02, `speed=${actual.speed}, pitch=${actual.pitch}, channel=${c}, RMS WASM=${actual.rms[c]}, native=${reference.rms[c]}`);
   }
  }
 }
 console.log(JSON.stringify({ iterations: 20, casesPerIteration: 5, fixedHeapBytes: initial.heapBytes, sourceBlockFrames: 512, maxOutputBlockFrames: 8192, elapsedMs: performance.now() - started, nativeSignalParity: true, scheduledFadeGainCases: 16, defaultFadeUnchanged: true, maxRMSDifference, rmsTolerance: .02 }, null, 2));
} finally { clearTimeout(timeout); await worker.terminate(); }
