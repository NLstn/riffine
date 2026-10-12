import { createOutputQueue, OutputWriter, QueueState, CountersBusyError } from './output-queue.mjs';
import { createPresentationTimeline } from './presentation-timeline.mjs';
import { PresentationMonitor } from './presentation-monitor.mjs';
import { OutputBoundary } from './output-boundary.mjs';
import { createPresentedCounter, PresentedWriter, PresentedState } from './presented-counter.mjs';
// Main-thread half of the device port. Source/DSP ownership is on the resource
// worker; this object owns only the context, callback gate and device timestamps.
export class LiveOutputHost {
 constructor(context, channels, onFailure) {
  if (typeof context.getOutputTimestamp !== 'function' || (channels !== 1 && channels !== 2)) throw new Error('Presentation-aware browser output is unavailable.');
  this.context = context; this.channels = channels; this.onFailure = onFailure;
  this.queue = undefined; this.writer = undefined; this.monitor = undefined;
  this.boundary = undefined; this.node = undefined; this.counter = undefined;
  this.closed = false; this.failure = undefined; this.closing = undefined;
  this.presentedState = PresentedState.quiescent;
 }
 async reset() {
  if (this.closed || this.failure) throw this.failure ?? new Error('Live output host has closed.');
  try {
   await this.suspend();
   if (this.closed || this.failure) throw this.failure ?? new Error('Live output host has closed.');
   this.releaseOutput();
   const queue = createOutputQueue(this.channels), timeline = createPresentationTimeline(), presented = createPresentedCounter();
   this.queue = queue; this.writer = new OutputWriter(queue); this.counter = new PresentedWriter(presented);
   this.presentedState = PresentedState.quiescent; this.counter.publish(0, this.presentedState);
   this.monitor = new PresentationMonitor(this.context, timeline, error => { void this.fail(error).catch(() => {}); }, frames => this.counter.publish(frames, this.presentedState), frames => this.checkOutput(frames));
   this.boundary = new OutputBoundary(this.context, queue, this.monitor);
   this.node = new AudioWorkletNode(this.context, 'riffine-output', {numberOfInputs: 0, numberOfOutputs: 1, outputChannelCount: [this.channels], processorOptions: {queue, timeline}});
   this.node.onprocessorerror = () => { void this.fail(new Error('Audio worklet failed.')).catch(() => {}); };
   this.node.connect(this.context.destination);
   return {queue, presented};
  } catch (error) { await this.fail(error); throw this.failure; }
 }
 async start() {
  if (this.closed || this.failure || !this.writer) throw this.failure ?? new Error('Live output host is unavailable.');
  try {
   if (this.writer.snapshot().state !== QueueState.primed) throw new Error('Live output must start from a fresh schedule generation.');
   await this.context.resume();
   if (this.closed || this.failure) throw this.failure ?? new Error('Live output host has closed.');
   this.monitor.progressAt = performance.now();
   this.presentedState = PresentedState.active; this.counter.publish(this.monitor.frames, this.presentedState);
   this.writer.start();
  } catch (error) { await this.fail(error); throw this.failure; }
 }
 async suspend() {
  if (this.failure) { await this.closing; throw this.failure; }
  if (this.closed) { await this.closingContext; return; }
  if (this.presentedState === PresentedState.quiescent && this.context.state === 'suspended') return;
  try {
   if (!this.writer || this.writer.snapshot().state === QueueState.primed) {
    await this.context.suspend(); // no submitted PCM to acknowledge
   } else {
    const paused = await this.boundary.pause();
    if (!paused) throw new Error('Unexpected terminal live output queue.');
   }
   if (this.writer && this.monitor.frames !== this.writer.snapshot().copiedFrames) throw new Error('Device pause boundary differs from presented PCM.');
   this.presentedState = PresentedState.quiescent;
   this.counter?.publish(this.monitor.frames, this.presentedState);
  } catch (error) { await this.fail(error); throw this.failure; }
 }
 checkOutput(frames) {
  if (this.presentedState !== PresentedState.active || this.closed || this.failure) return;
  let snapshot;
  try { snapshot = this.writer.snapshot(); } catch (error) { if (error instanceof CountersBusyError) return; throw error; }
  if (snapshot.state === QueueState.error || snapshot.state === QueueState.closed) throw new Error(this.writer.failureMessage());
  if (snapshot.copiedFrames > frames && performance.now() - this.monitor.progressAt > 10000) throw new Error('Live device presentation stopped advancing.');
 }
 releaseOutput() {
  this.boundary?.close(); this.monitor?.close(); this.node?.disconnect();
  this.boundary = undefined; this.monitor = undefined; this.node = undefined;
 }
 async fail(error) {
  if (!this.failure) {
   this.failure = error instanceof Error ? error : new Error(String(error));
   this.releaseOutput();
   this.closing = this.closeContext().then(() => {
    if (this.writer?.failureCode() && !this.failure.message.includes(this.writer.failureMessage())) this.failure = new Error(`${this.failure.message}; ${this.writer.failureMessage()}`);
    this.counter?.publish(this.counter.frames, PresentedState.failed);
    this.onFailure(this.failure);
   });
   // All callers await this fence. Keep its notification failure handled while
   // an asynchronous monitor/worklet error requests teardown independently.
   void this.closing.catch(() => {});
  }
  await this.closing;
 }
 closeContext() {
  if (!this.closingContext) this.closingContext = (async () => {
   if (this.context.state === 'closed') return;
   try { await this.context.close(); }
   catch (closeError) {
    if (this.context.state === 'closed') return;
    // A rejected close is not a quiescence acknowledgement. The callback gate
    // is already closed; require the context's suspension fence before exposing
    // failure to the worker or releasing its borrowed-source callbacks.
    try { await this.context.suspend(); }
    catch (suspendError) { throw new AggregateError([closeError, suspendError], 'Live context could not quiesce.'); }
   }
  })();
  return this.closingContext;
 }
 async close() {
  if (this.closed) { await this.closingContext; return; }
  this.closed = true; this.releaseOutput();
  await this.closeContext();
  if (!this.failure) this.counter?.publish(this.counter.frames, PresentedState.quiescent);
 }
}
