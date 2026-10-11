import { Selection } from './selection.mjs';
import { OutputWriter, QueueState, CountersBusyError } from './output-queue.mjs';
import { PresentedReader, PresentedBusyError, PresentedState } from './presented-counter.mjs';
import { ScheduleAcknowledgements } from './schedule-acknowledgements.mjs';
const delay = () => new Promise(resolve => setTimeout(resolve, 4));
// Resource/DSP-worker adapter. The caller supplies verified decoded source reads
// and owns presentation-aware device lifecycle. This class never chooses loops,
// adjusts source positions, extrapolates device clocks or emits practice events.
export class ScheduledSelections {
 constructor(module, info, readPCM, onFailure) {
  if (typeof readPCM !== 'function' || typeof onFailure !== 'function') throw new Error('Scheduled processing requires source and failure ports.');
  this.module = module; this.info = Object.freeze({...info}); this.readPCM = readPCM; this.onFailure = onFailure;
  this.ledger = new ScheduleAcknowledgements(); this.jobs = []; this.epoch = 0; this.task = undefined;
  this.writer = undefined; this.counter = undefined; this.failure = undefined; this.closed = false; this.queuedFrames = 0;
 }
 // Caller must quiesce the old device first. This is a processing/resource fence,
 // not a hardware suspend operation. Fresh queue/counter identify the new epoch.
 async reset(generation, queue, counter) {
  if (this.closed) throw new Error('Scheduled processing has closed.');
  await this.stopProcessing();
  if (this.closed) throw new Error('Scheduled processing has closed.');
  const writer = new OutputWriter(queue), reader = new PresentedReader(counter);
  const initial = reader.snapshot();
  if (writer.channels !== this.info.channels || writer.snapshot().copiedFrames !== 0 || writer.snapshot().state !== QueueState.primed || initial.frames !== 0) throw new Error('Reset requires fresh output buffers.');
  this.ledger.reset(generation); this.writer = writer; this.counter = reader; this.failure = undefined; this.queuedFrames = 0;
 }
 schedule(schedule) {
  if (this.closed || !this.writer || this.failure) throw this.failure ?? new Error('Scheduled processing is unavailable.');
  let output;
  try { output = this.writer.snapshot(); } catch (error) { if (!(error instanceof CountersBusyError)) throw error; }
  if (this.ledger.outputEnd && output && output.copiedFrames >= this.ledger.outputEnd && output.underrunFrames) {
   const error = new Error('Output starved before a scheduled continuation.'); this.fail(error); throw error;
  }
  const s = {...schedule};
  if (s.source !== this.info.source || !Number.isSafeInteger(s.startFrame) || !Number.isSafeInteger(s.endFrame) || s.startFrame < 0 || s.endFrame <= s.startFrame || s.endFrame > this.info.frames) throw new Error('Invalid verified source schedule.');
  const selection = new Selection(this.module, this.info.channels, this.info.sampleRate, s.endFrame - s.startFrame, s.speed, s.pitch, s.edgeFadeFrames);
  try {
   if (selection.info().outputFrames !== s.outputEnd - s.outputStart) throw new Error('Scheduled output length differs from shared processing.');
   this.ledger.accept(s);
   this.jobs.push({schedule: Object.freeze(s), selection, read: 0});
  } catch (error) { selection.close(); throw error; }
  this.ensurePump();
 }
 ensurePump() {
  if (this.task || !this.jobs.length || this.failure) return;
  const epoch = this.epoch;
  this.task = this.pump(epoch).catch(error => { if (epoch === this.epoch) this.fail(error); }).finally(() => {
   this.task = undefined;
   if (epoch === this.epoch && !this.failure) this.ensurePump();
  });
 }
 async pump(epoch) {
  while (this.jobs.length && epoch === this.epoch) {
   const job = this.jobs[0], s = job.schedule;
   try {
    for (;;) {
     if (epoch !== this.epoch) return;
     const result = job.selection.read(4096);
     if (result.frames) {
      for (;;) { if (epoch !== this.epoch) return; if (this.writer.write(result.pcm)) break; await delay(); }
      this.queuedFrames += result.frames;
     }
     if (result.needsInput) {
      const count = Math.min(512, s.endFrame - s.startFrame - job.read);
      if (count < 1) throw new Error('Scheduled processing requested source beyond its range.');
      const pcm = await this.readPCM(s.startFrame + job.read, count);
      if (epoch !== this.epoch) return;
      if (!(pcm instanceof Float32Array) || pcm.length !== count * this.info.channels) throw new Error('Decoded schedule block has the wrong dimensions.');
      job.selection.push(pcm); job.read += count;
     } else if (!result.frames) {
      if (job.read !== s.endFrame - s.startFrame || this.queuedFrames !== s.outputEnd) throw new Error('Scheduled processing duration mismatch.');
      break;
     }
    }
   } finally { job.selection.close(); }
   this.jobs.shift();
  }
 }
 async prefilled() {
  const started = performance.now();
  for (;;) {
   if (this.failure) throw this.failure;
   if (this.closed || !this.writer) throw new Error('Scheduled processing has closed.');
   let output;
   try { output = this.writer.snapshot(); } catch (error) { if (!(error instanceof CountersBusyError)) throw error; }
   const queued = output ? this.queuedFrames - output.copiedFrames : 0;
   const first = this.ledger.entries[0];
   if (first && queued >= Math.min(4096, first.outputEnd - first.outputStart)) return;
   if (performance.now() - started > 10000) throw new Error('Scheduled processing did not prefill output.');
   await delay();
  }
 }
 drain(limit) {
  if (this.closed || !this.counter) throw new Error('Scheduled processing has closed.');
  let snapshot;
  try { snapshot = this.counter.snapshot(); }
  catch (error) { if (error instanceof PresentedBusyError) return []; this.fail(error); return []; }
  // Caller publishes failed only AFTER device quiescence. A worker failure
  // closes output and requests host cleanup; until its fence completes, do not
  // expose an error that could release the verified reader prematurely.
  if (snapshot.state === PresentedState.failed) throw this.failure ?? new Error('Device output failed.');
  if (this.failure) return [];
  try {
   const output = this.writer.snapshot();
   if (output.state === QueueState.error || output.underrunFrames && output.copiedFrames < this.ledger.outputEnd) throw new Error('Output starved while scheduled PCM was pending.');
   return this.ledger.drain(snapshot.frames, limit);
  }
  catch (error) { if (error instanceof CountersBusyError) return []; this.fail(error); return []; }
 }
 fail(error) {
  if (this.failure) return;
  this.failure = error instanceof Error ? error : new Error(String(error));
  this.writer?.close(); this.epoch++;
  // Notification runs on this control worker. Host owns the asynchronous device
  // stop fence and failed-counter publication; never call core from this hook.
  try { this.onFailure(this.failure); } catch (notificationError) { this.failure = new AggregateError([this.failure, notificationError], 'Device failure notification failed.'); }
 }
 async stopProcessing() {
  this.epoch++; this.writer?.close();
  await this.task;
  for (const job of this.jobs) job.selection.close(); this.jobs = [];
 }
 async close() { if (!this.closing) { this.closed = true; this.closing = this.stopProcessing(); } await this.closing; }
}
