// A processing-worker adapter to the shared C++ single-selection ABI. No project,
// loop, presentation-clock or practice-counter policy lives in this adapter.
export class Selection {
 constructor(module, channels, sampleRate, sourceFrames, speed, pitch, edgeFadeFrames, outputRate = sampleRate) {
  this.module = module;
  this.channels = channels;
  this.rateConverted = outputRate !== sampleRate;
  this.api = this.rateConverted ? '_riffine_rate_selection' : '_riffine_selection';
  this.handle = 0;
  this.buffers = [];
  this.closed = false;
  const allocate = bytes => {
   const pointer = module._malloc(bytes);
   if (!pointer) throw new Error('Processing worker exhausted its fixed heap.');
   this.buffers.push(pointer);
   return pointer;
  };
  try {
   if (!Number.isInteger(outputRate) || outputRate < 8000 || outputRate > 192000) throw new Error('Unsupported device output rate.');
   this.error = allocate(512);
   this.result = allocate(8);
   this.infoBuffer = allocate(this.rateConverted ? 32 : 24);
   // Channel count is checked by C++ before allocating the PCM buffers.
   const status = this.rateConverted
    ? module._riffine_rate_selection_create(channels, sampleRate, outputRate, BigInt(sourceFrames), speed, pitch, this.result, this.error, 512)
    : module._riffine_selection_create(channels, sampleRate, BigInt(sourceFrames), speed, pitch, this.result, this.error, 512);
   this.check(status);
   this.handle = this.view().getUint32(this.result, true);
   if (edgeFadeFrames !== undefined) {
    if (!Number.isSafeInteger(edgeFadeFrames) || edgeFadeFrames < 0) throw new Error('Invalid scheduled edge fade.');
    this.check(module[`${this.api}_set_edge_fade_frames`](this.handle, BigInt(edgeFadeFrames), this.error, 512));
   }
   this.input = allocate(512 * channels * 4);
   this.output = allocate(8192 * channels * 4);
   this.info();
  } catch (error) { this.close(); throw error; }
 }
 view() { return new DataView(this.module.HEAPU8.buffer); }
 check(status) {
  if (!status) return;
  const bytes = this.module.HEAPU8.subarray(this.error, this.error + 512);
  const end = bytes.indexOf(0);
  throw new Error(new TextDecoder().decode(bytes.subarray(0, end < 0 ? 512 : end)) || 'Shared processing failed.');
 }
 usable() { if (this.closed) throw new Error('Selection processor has closed.'); }
 info() {
  this.usable();
  this.check(this.module[`${this.api}_info_get`](this.handle, this.infoBuffer, this.error, 512));
  const view = this.view(), pointer = this.infoBuffer;
  if (this.rateConverted) {
   const outputFrames = Number(view.getBigUint64(pointer + 16, true));
   if (!Number.isSafeInteger(outputFrames)) throw new Error('Device output exceeds safe worker coordinates.');
   return {channels: view.getUint32(pointer, true), sourceSampleRate: view.getUint32(pointer + 4, true), sampleRate: view.getUint32(pointer + 8, true),
    resamplerLatencyFrames: view.getUint32(pointer + 12, true), outputFrames, consumedSourceFrames: Number(view.getBigUint64(pointer + 24, true))};
  }
  return { channels: view.getUint32(pointer, true), sampleRate: view.getUint32(pointer + 4, true),
   outputFrames: Number(view.getBigUint64(pointer + 8, true)), consumedSourceFrames: Number(view.getBigUint64(pointer + 16, true)) };
 }
 push(input) {
  this.usable();
  if (!(input instanceof Float32Array) || input.length < this.channels || input.length > 512 * this.channels || input.length % this.channels) throw new Error('Expected 1–512 whole input frames.');
  this.module.HEAPF32.set(input, this.input / 4);
  this.check(this.module[`${this.api}_push`](this.handle, this.input, input.length / this.channels, this.error, 512));
 }
 read(capacity = 8192) {
  this.usable();
  if (!Number.isInteger(capacity) || capacity < 1 || capacity > 8192) throw new Error('Output capacity must be 1–8192 frames.');
  this.check(this.module[`${this.api}_read`](this.handle, this.output, capacity, this.result, this.result + 4, this.error, 512));
  const view = this.view(), frames = view.getUint32(this.result, true);
  if (frames > capacity) throw new Error('Shared processing exceeded output capacity.');
  const needsInput = view.getUint32(this.result + 4, true) !== 0;
  return { pcm: this.module.HEAPF32.slice(this.output / 4, this.output / 4 + frames * this.channels), frames, needsInput };
 }
 close() {
  if (this.closed) return;
  this.closed = true;
  if (this.handle) this.module[`${this.api}_destroy`](this.handle);
  this.handle = 0;
  for (const buffer of this.buffers) this.module._free(buffer);
  this.buffers = [];
 }
}
