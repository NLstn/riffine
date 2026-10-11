// One main/control writer publishes mapped device PCM position to its resource
// worker. This is never written by the audio callback or advanced by a UI clock.
const words = 8, range = 4294967296;
export const PresentedState = Object.freeze({ active: 0, quiescent: 1, failed: 2 });
export class PresentedBusyError extends Error {}
export function createPresentedCounter() {
 const buffer = new SharedArrayBuffer(words * 4); new Int32Array(buffer)[0] = 1; return buffer;
}
class Counter {
 constructor(buffer) {
  if (!(buffer instanceof SharedArrayBuffer) || buffer.byteLength !== words * 4) throw new Error('Invalid presented counter.');
  this.data = new Int32Array(buffer);
  if (Atomics.load(this.data, 0) !== 1) throw new Error('Unsupported presented counter.');
 }
}
export class PresentedWriter extends Counter {
 constructor(buffer) { super(buffer); this.frames = 0; }
 publish(frames, state = PresentedState.active) {
  if (!Number.isSafeInteger(frames) || frames < this.frames || !Number.isInteger(state) || state < 0 || state > 2) throw new Error('Invalid device presentation update.');
  Atomics.add(this.data, 1, 1);
  Atomics.store(this.data, 2, frames | 0); Atomics.store(this.data, 3, Math.floor(frames / range));
  Atomics.store(this.data, 4, state); Atomics.add(this.data, 1, 1); this.frames = frames;
 }
}
export class PresentedReader extends Counter {
 snapshot() {
  for (let attempt = 0; attempt < 8; attempt++) {
   const before = Atomics.load(this.data, 1); if (before & 1) continue;
   const frames = (Atomics.load(this.data, 2) >>> 0) + (Atomics.load(this.data, 3) >>> 0) * range;
   const state = Atomics.load(this.data, 4);
   if (Atomics.load(this.data, 1) !== before) continue;
   if (!Number.isSafeInteger(frames) || state < 0 || state > 2) throw new Error('Corrupt device presentation update.');
   return { frames, state };
  }
  throw new PresentedBusyError('Device presentation update is busy.');
 }
}
