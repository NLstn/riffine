import { PresentationWriter } from './presentation-timeline.mjs';
// Device-buffer plumbing only. Queue consumption counts callback submission;
// it must never be passed to the shared core as hardware presentation evidence.
export const QueueState = Object.freeze({ primed: 0, running: 1, closed: 2, error: 3, done: 4, pausing: 5, paused: 6 });
export class CountersBusyError extends Error {}
const words = 16, bytes = words * 4;
const version = 0, channelsIndex = 1, capacityIndex = 2, writeIndex = 3, readIndex = 4,
 stateIndex = 5, eofIndex = 6, copiedLow = 7, copiedHigh = 8, counterSequence = 9,
 underrunLow = 10, underrunHigh = 11;
const uint32Range = 4294967296;

export function createOutputQueue(channels, capacity = 32768) {
 if ((channels !== 1 && channels !== 2) || !Number.isInteger(capacity) || capacity < 128 || capacity > 65536 || (capacity & (capacity - 1))) throw new Error('Invalid output queue dimensions.');
 const buffer = new SharedArrayBuffer(bytes + capacity * channels * 4);
 const header = new Int32Array(buffer, 0, words);
 header[version] = 1; header[channelsIndex] = channels; header[capacityIndex] = capacity;
 return buffer;
}
class Queue {
 constructor(buffer) {
  if (!(buffer instanceof SharedArrayBuffer) || buffer.byteLength < bytes) throw new Error('Expected shared output queue.');
  this.header = new Int32Array(buffer, 0, words);
  this.channels = Atomics.load(this.header, channelsIndex);
  this.capacity = Atomics.load(this.header, capacityIndex);
  if (Atomics.load(this.header, version) !== 1 || (this.channels !== 1 && this.channels !== 2) || this.capacity < 128 || this.capacity > 65536 || (this.capacity & (this.capacity - 1)) || buffer.byteLength !== bytes + this.capacity * this.channels * 4) throw new Error('Invalid shared output queue.');
  this.pcm = new Float32Array(buffer, bytes);
  this.mask = this.capacity - 1;
 }
 close() { Atomics.store(this.header, stateIndex, QueueState.closed); }
}
// One processing/control worker owns all writes and EOF. Another control thread
// may start/close the queue; no producer writes a reader cursor or counter.
export class OutputWriter extends Queue {
 write(pcm) {
  if (!(pcm instanceof Float32Array) || pcm.length < this.channels || pcm.length > 8192 * this.channels || pcm.length % this.channels) throw new Error('Invalid output PCM block.');
  const state = Atomics.load(this.header, stateIndex);
  if (state !== QueueState.primed && state !== QueueState.running && state !== QueueState.pausing && state !== QueueState.paused || Atomics.load(this.header, eofIndex)) throw new Error('Output queue no longer accepts PCM.');
  const write = Atomics.load(this.header, writeIndex) >>> 0;
  const read = Atomics.load(this.header, readIndex) >>> 0;
  const used = (write - read) >>> 0;
  if (used > this.capacity) throw new Error('Output queue cursors are corrupt.');
  const frames = pcm.length / this.channels;
  if (frames > this.capacity - used) return false;
  for (let i = 0; i < pcm.length; i++) if (!Number.isFinite(pcm[i])) throw new Error('Nonfinite output PCM.');
  for (let frame = 0; frame < frames; frame++) {
   const offset = ((write + frame) & this.mask) * this.channels;
   for (let c = 0; c < this.channels; c++) this.pcm[offset + c] = pcm[frame * this.channels + c];
  }
  // Publish only after the complete block; consumer never reads unowned cells.
  Atomics.store(this.header, writeIndex, (write + frames) | 0);
  return true;
 }
 start() {
  if (Atomics.compareExchange(this.header, stateIndex, QueueState.primed, QueueState.running) !== QueueState.primed) throw new Error('Output queue cannot start again.');
 }
 requestPause() {
  const state = Atomics.load(this.header, stateIndex);
  if (state === QueueState.running) Atomics.compareExchange(this.header, stateIndex, QueueState.running, QueueState.pausing);
 }
 resume() {
  if (Atomics.compareExchange(this.header, stateIndex, QueueState.paused, QueueState.running) !== QueueState.paused) throw new Error('Output queue is not paused.');
 }
 sealEOF() { Atomics.store(this.header, eofIndex, 1); }
 snapshot() {
  // Only the audio consumer writes these counters. Bounded seqlock reads avoid
  // a torn low/high pair without blocking or spinning on the audio thread.
  for (let attempt = 0; attempt < 8; attempt++) {
   const before = Atomics.load(this.header, counterSequence);
   if (before & 1) continue;
   const copied = (Atomics.load(this.header, copiedLow) >>> 0) + (Atomics.load(this.header, copiedHigh) >>> 0) * uint32Range;
   const underruns = (Atomics.load(this.header, underrunLow) >>> 0) + (Atomics.load(this.header, underrunHigh) >>> 0) * uint32Range;
   if (Atomics.load(this.header, counterSequence) === before) return { copiedFrames: copied, underrunFrames: underruns, state: Atomics.load(this.header, stateIndex) };
  }
  throw new CountersBusyError('Output counters are being updated; retry on the control worker.');
 }
}

// Construct on the worklet's setup path. process only copies/zeros PCM and
// updates fixed atomic integer fields: no allocation, locks, waits, messages,
// decoding, DSP, logging, Go calls or practice policy on the audio callback.
export class OutputConsumer extends Queue {
 constructor(buffer, timeline) { super(buffer); this.timeline = timeline ? new PresentationWriter(timeline) : undefined; }
 addCounter(lowIndex, highIndex, increment) {
  const low = Atomics.load(this.header, lowIndex) >>> 0;
  const sum = low + increment;
  Atomics.store(this.header, lowIndex, sum | 0);
  if (sum >= uint32Range) Atomics.add(this.header, highIndex, 1);
 }
 process(outputs, contextFrame) {
  if (!outputs) { Atomics.store(this.header, stateIndex, QueueState.error); return false; }
  for (let c = 0; c < outputs.length; c++) outputs[c].fill(0);
  const state = Atomics.load(this.header, stateIndex);
  // A pause request is acknowledged only on a callback boundary, after any
  // preceding copy/counters/timeline publication. Keep the context graph alive
  // with silence so device timestamps can catch up to submitted PCM.
  if (state === QueueState.pausing) { Atomics.store(this.header, stateIndex, QueueState.paused); return true; }
  if (state !== QueueState.running) return state === QueueState.primed || state === QueueState.paused;
  if (outputs.length !== this.channels || !outputs[0] || !outputs[0].length) { Atomics.store(this.header, stateIndex, QueueState.error); return false; }
  const quantum = outputs[0].length;
  for (let c = 1; c < this.channels; c++) if (outputs[c].length !== quantum) { Atomics.store(this.header, stateIndex, QueueState.error); return false; }
  const read = Atomics.load(this.header, readIndex) >>> 0;
  const write = Atomics.load(this.header, writeIndex) >>> 0;
  const available = (write - read) >>> 0;
  if (available > this.capacity) { Atomics.store(this.header, stateIndex, QueueState.error); return false; }
  const count = Math.min(available, quantum);
  if (count && this.timeline && !this.timeline.canAppend(contextFrame, count)) { Atomics.store(this.header, stateIndex, QueueState.error); return false; }
  for (let frame = 0; frame < count; frame++) {
   const offset = ((read + frame) & this.mask) * this.channels;
   for (let c = 0; c < this.channels; c++) outputs[c][frame] = this.pcm[offset + c];
  }
  if (count && this.timeline) this.timeline.append(contextFrame, count);
  Atomics.store(this.header, readIndex, (read + count) | 0);
  // Reload EOF after acquiring the producer's write cursor. A producer that
  // seals concurrently may have published more PCM; recheck before declaring done.
  const eof = Atomics.load(this.header, eofIndex);
  const finalWrite = eof ? Atomics.load(this.header, writeIndex) >>> 0 : write;
  Atomics.add(this.header, counterSequence, 1);
  this.addCounter(copiedLow, copiedHigh, count);
  const remaining = (finalWrite - ((read + count) >>> 0)) >>> 0;
  if (count < quantum && (!eof || remaining > 0)) this.addCounter(underrunLow, underrunHigh, quantum - count);
  Atomics.add(this.header, counterSequence, 1);
  if (eof && remaining === 0) { Atomics.store(this.header, stateIndex, QueueState.done); return false; }
  return true;
 }
}
