import { OutputWriter, QueueState, CountersBusyError } from './output-queue.mjs';
const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds));
// Main-thread device lifecycle only. No practice policy. The context stays
// running with silence until all copied PCM through the callback gate is past
// the output timestamp; suspend's hardware handoff promise is not an audio ack.
export class OutputBoundary {
 constructor(context, queue, presentation) {
  this.context = context; this.writer = new OutputWriter(queue); this.presentation = presentation;
  this.busy = false; this.paused = false; this.closed = false;
  this.cancelled = new Promise((_resolve, reject) => { this.abort = reject; });
  void this.cancelled.catch(() => {});
 }
 async wait(action) { return Promise.race([action, this.cancelled]); }
 async pause() {
  if (this.closed) throw new Error('Audio output boundary closed.');
  if (this.busy) throw new Error('Another audio boundary operation is running.');
  if (this.paused) return true;
  this.busy = true;
  const started = performance.now();
  try {
   let boundary;
   for (;;) {
    if (this.closed) throw new Error('Audio output boundary closed.');
    let state;
    try { state = this.writer.snapshot(); } catch (error) { if (!(error instanceof CountersBusyError)) throw error; }
    if (state?.state === QueueState.done) return false; // finite preview already ended
    if (state?.state === QueueState.closed || state?.state === QueueState.error) throw new Error('Audio output stopped before its pause boundary.');
    if (state?.state === QueueState.running) this.writer.requestPause();
    if (state?.state === QueueState.paused) { boundary = state.copiedFrames; break; }
    if (performance.now() - started > 10000) throw new Error('Audio callback did not acknowledge its pause boundary.');
    await this.wait(delay(4));
   }
   await this.wait(this.presentation.waitForFrames(boundary));
   await this.wait(this.context.suspend());
   this.paused = true;
   return true;
  } finally { this.busy = false; }
 }
 async resume() {
  if (this.closed) throw new Error('Audio output boundary closed.');
  if (this.busy) throw new Error('Another audio boundary operation is running.');
  if (!this.paused) return;
  this.busy = true;
  try {
   // Invoke immediately from the user's resume action, before the first await.
   await this.wait(this.context.resume());
   this.writer.resume(); this.paused = false;
  } finally { this.busy = false; }
 }
 close() {
  if (this.closed) return;
  this.closed = true; this.writer.close(); this.abort(new Error('cancelled: audio output boundary closed'));
 }
}
