// Context-frame -> copied-PCM mapping only. No source positions, loops or UI clocks.
// One AudioWorklet writer and one control-thread reader; entries are immutable
// until the reader releases them. No messages, allocation or waits in append.
const headerWords = 8, entryWords = 5, range = 4294967296;
const versionIndex = 0, capacityIndex = 1, writeIndex = 2, readIndex = 3, errorIndex = 4;
export class TimelineError extends Error {}

export function createPresentationTimeline(capacity = 1024) {
 if (!Number.isInteger(capacity) || capacity < 2 || capacity > 4096 || (capacity & (capacity - 1))) throw new TimelineError('Invalid presentation timeline capacity.');
 const buffer = new SharedArrayBuffer((headerWords + entryWords * capacity) * 4);
 const data = new Int32Array(buffer);
 data[versionIndex] = 1; data[capacityIndex] = capacity;
 return buffer;
}
class Timeline {
 constructor(buffer) {
  if (!(buffer instanceof SharedArrayBuffer) || buffer.byteLength < headerWords * 4) throw new TimelineError('Expected shared presentation timeline.');
  this.data = new Int32Array(buffer);
  this.capacity = Atomics.load(this.data, capacityIndex);
  if (Atomics.load(this.data, versionIndex) !== 1 || this.capacity < 2 || this.capacity > 4096 || (this.capacity & (this.capacity - 1)) || buffer.byteLength !== (headerWords + entryWords * this.capacity) * 4) throw new TimelineError('Invalid shared presentation timeline.');
  this.mask = this.capacity - 1;
 }
 fail() { Atomics.store(this.data, errorIndex, 1); }
}
export class PresentationWriter extends Timeline {
 constructor(buffer) { super(buffer); this.totalFrames = 0; this.contextEnd = 0; }
 // Call before copying PCM. Exhaustion/malformed coordinates fail closed, rather
 // than lose an underrun gap and subsequently acknowledge the wrong audio.
 canAppend(contextFrame, count) {
  if (Atomics.load(this.data, errorIndex)) return false;
  const write = Atomics.load(this.data, writeIndex) >>> 0;
  const read = Atomics.load(this.data, readIndex) >>> 0;
  if (!Number.isSafeInteger(contextFrame) || contextFrame < this.contextEnd || !Number.isInteger(count) || count < 1 || count > 8192 || !Number.isSafeInteger(contextFrame + count) || !Number.isSafeInteger(this.totalFrames + count) || ((write - read) >>> 0) >= this.capacity) { this.fail(); return false; }
  return true;
 }
 // Sole writer calls only after canAppend succeeded and PCM was copied. Reader
 // can free capacity concurrently, but cannot invalidate this writer's slot.
 append(contextFrame, count) {
  const write = Atomics.load(this.data, writeIndex) >>> 0;
  const offset = headerWords + (write & this.mask) * entryWords;
  Atomics.store(this.data, offset, contextFrame | 0);
  Atomics.store(this.data, offset + 1, Math.floor(contextFrame / range));
  Atomics.store(this.data, offset + 2, count);
  Atomics.store(this.data, offset + 3, this.totalFrames | 0);
  Atomics.store(this.data, offset + 4, Math.floor(this.totalFrames / range));
  this.totalFrames += count; this.contextEnd = contextFrame + count;
  Atomics.store(this.data, writeIndex, (write + 1) | 0);
 }
}
export class PresentationReader extends Timeline {
 constructor(buffer, sampleRate) {
  super(buffer);
  if (!Number.isInteger(sampleRate) || sampleRate < 8000 || sampleRate > 192000) throw new TimelineError('Invalid presentation sample rate.');
  this.sampleRate = sampleRate; this.presentedFrames = 0; this.completedFrames = 0;
  this.contextEnd = 0; this.timestampFrame = 0;
 }
 // contextTime is exclusively AudioContext.getOutputTimestamp().contextTime.
 // Count frames strictly BEFORE that device position: the currently rendered
 // frame is not yet acknowledged. Never extrapolate using performance.now().
 observe(timestamp) {
  if (Atomics.load(this.data, errorIndex)) throw new TimelineError('Presentation timeline failed.');
  const frame = Math.floor(timestamp?.contextTime * this.sampleRate);
  if (!Number.isFinite(timestamp?.contextTime) || timestamp.contextTime < 0 || !Number.isFinite(timestamp?.performanceTime) || timestamp.performanceTime < 0 || !Number.isSafeInteger(frame)) return this.reject('Invalid output timestamp.');
  // Device timestamp estimates can move backwards during startup or adjustment.
  // Treat them as stale observations: preserve progress without examining new
  // entries or extrapolating until the reported device position catches up.
  if (frame < this.timestampFrame) return this.presentedFrames;
  this.timestampFrame = frame;
  const write = Atomics.load(this.data, writeIndex) >>> 0;
  let read = Atomics.load(this.data, readIndex) >>> 0;
  const available = (write - read) >>> 0;
  if (available > this.capacity) return this.reject('Corrupt presentation timeline cursors.');
  // A bounded snapshot of published entries. Producer may append concurrently;
  // the next observation acquires those entries rather than chasing its cursor.
  for (let i = 0; i < available; i++) {
   const offset = headerWords + (read & this.mask) * entryWords;
   const start = (Atomics.load(this.data, offset) >>> 0) + (Atomics.load(this.data, offset + 1) >>> 0) * range;
   const count = Atomics.load(this.data, offset + 2);
   const ordinal = (Atomics.load(this.data, offset + 3) >>> 0) + (Atomics.load(this.data, offset + 4) >>> 0) * range;
   if (!Number.isSafeInteger(start + count) || start < this.contextEnd || count < 1 || count > 8192 || ordinal !== this.completedFrames || !Number.isSafeInteger(ordinal + count)) return this.reject('Corrupt presentation timeline entry.');
   if (frame <= start) break;
   this.presentedFrames = ordinal + Math.min(count, frame - start);
   if (frame < start + count) break;
   this.completedFrames = ordinal + count; this.contextEnd = start + count;
   read = (read + 1) >>> 0;
   Atomics.store(this.data, readIndex, read | 0);
  }
  return this.presentedFrames;
 }
 reject(message) { this.fail(); throw new TimelineError(message); }
}
