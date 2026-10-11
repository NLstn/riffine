// A processing-worker adapter to the shared C++ single-selection ABI. No project,
// loop, presentation-clock or practice-counter policy lives in this adapter.
export class Selection {
 constructor(module, channels, sampleRate, sourceFrames, speed, pitch) {
  this.module = module;
  this.channels = channels;
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
   this.error = allocate(512);
   this.result = allocate(8);
   this.infoBuffer = allocate(24);
   // Channel count is checked by C++ before allocating the PCM buffers.
   const status = module._riffine_selection_create(channels, sampleRate,
    BigInt(sourceFrames), speed, pitch, this.result, this.error, 512);
   this.check(status);
   this.handle = this.view().getUint32(this.result, true);
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
  this.check(this.module._riffine_selection_info_get(this.handle, this.infoBuffer, this.error, 512));
  const view = this.view(), pointer = this.infoBuffer;
  return { channels: view.getUint32(pointer, true), sampleRate: view.getUint32(pointer + 4, true),
   outputFrames: Number(view.getBigUint64(pointer + 8, true)), consumedSourceFrames: Number(view.getBigUint64(pointer + 16, true)) };
 }
 push(input) {
  this.usable();
  if (!(input instanceof Float32Array) || input.length < this.channels || input.length > 512 * this.channels || input.length % this.channels) throw new Error('Expected 1–512 whole input frames.');
  this.module.HEAPF32.set(input, this.input / 4);
  this.check(this.module._riffine_selection_push(this.handle, this.input, input.length / this.channels, this.error, 512));
 }
 read(capacity = 8192) {
  this.usable();
  if (!Number.isInteger(capacity) || capacity < 1 || capacity > 8192) throw new Error('Output capacity must be 1–8192 frames.');
  this.check(this.module._riffine_selection_read(this.handle, this.output, capacity, this.result, this.result + 4, this.error, 512));
  const view = this.view(), frames = view.getUint32(this.result, true);
  if (frames > capacity) throw new Error('Shared processing exceeded output capacity.');
  const needsInput = view.getUint32(this.result + 4, true) !== 0;
  return { pcm: this.module.HEAPF32.slice(this.output / 4, this.output / 4 + frames * this.channels), frames, needsInput };
 }
 close() {
  if (this.closed) return;
  this.closed = true;
  if (this.handle) this.module._riffine_selection_destroy(this.handle);
  this.handle = 0;
  for (const buffer of this.buffers) this.module._free(buffer);
  this.buffers = [];
 }
}
