import { PresentationReader, TimelineError } from './presentation-timeline.mjs';
// Control-thread sampling only. The timer requests fresh device timestamps; it
// never advances audio position, estimates missing frames or extrapolates time.
export class PresentationMonitor {
 constructor(context, timeline, onError, onProgress, onSample) {
  if (typeof context.getOutputTimestamp !== 'function') throw new TimelineError('Output timestamps are unavailable.');
  this.context = context; this.reader = new PresentationReader(timeline, context.sampleRate);
  this.onError = onError; this.onProgress = onProgress; this.onSample = onSample; this.target = undefined; this.failure = undefined;
  this.closed = false; this.progressAt = performance.now(); this.frames = 0;
  this.timer = setInterval(() => this.sample(), 20);
 }
 sample() {
  if (this.closed || this.failure) return;
  try {
   const frames = this.reader.observe(this.context.getOutputTimestamp());
   if (frames !== this.frames) { this.frames = frames; this.progressAt = performance.now(); this.onProgress?.(frames); }
   this.onSample?.(frames);
   if (this.target !== undefined) {
    if (frames > this.target) throw new TimelineError('Presented audio exceeds the preview output.');
    if (frames === this.target) { this.resolve?.(frames); this.resolve = undefined; this.reject = undefined; this.target = undefined; }
    else if (performance.now() - this.progressAt > 10000) throw new TimelineError('Audio output timestamp stopped advancing.');
   }
  } catch (error) {
   this.failure = error instanceof Error ? error : new TimelineError(String(error));
   clearInterval(this.timer); this.reject?.(this.failure); this.resolve = undefined; this.reject = undefined;
   this.onError(this.failure);
  }
 }
 finish(expectedFrames) { return this.waitForFrames(expectedFrames); }
 waitForFrames(expectedFrames) {
  if (this.closed) return Promise.reject(new TimelineError('Presentation monitor closed.'));
  if (this.failure) return Promise.reject(this.failure);
  if (this.target !== undefined || !Number.isSafeInteger(expectedFrames) || expectedFrames < 0) return Promise.reject(new TimelineError('Invalid presentation completion target.'));
  this.target = expectedFrames; this.progressAt = performance.now();
  const done = new Promise((resolve, reject) => { this.resolve = resolve; this.reject = reject; });
  this.sample();
  return done;
 }
 close() {
  if (this.closed) return;
  this.closed = true; clearInterval(this.timer);
  this.reject?.(new TimelineError('Presentation monitor closed.'));
  this.resolve = undefined; this.reject = undefined;
 }
}
