import assert from 'node:assert/strict';
import { setTimeout as delay } from 'node:timers/promises';
import { createOutputQueue, OutputWriter, OutputConsumer, QueueState } from '../web/public/practice/output-queue.mjs';
import { createPresentationTimeline } from '../web/public/practice/presentation-timeline.mjs';
import { PresentationMonitor } from '../web/public/practice/presentation-monitor.mjs';
import { OutputBoundary } from '../web/public/practice/output-boundary.mjs';
const queue = createOutputQueue(1, 512), timeline = createPresentationTimeline(8);
const writer = new OutputWriter(queue), consumer = new OutputConsumer(queue, timeline);
const output = [new Float32Array(128)];
let deviceFrame = 0;
const calls = [];
const context = { sampleRate: 48000, getOutputTimestamp: () => ({contextTime: (deviceFrame + .125) / 48000, performanceTime: 1}), suspend: async () => { calls.push('suspend'); }, resume: async () => { calls.push('resume'); } };
const monitor = new PresentationMonitor(context, timeline, error => { throw error; });
const boundary = new OutputBoundary(context, queue, monitor);
writer.write(new Float32Array(256).map((_v, i) => (i + 1) / 1024)); writer.start();
consumer.process(output, 1024);
let paused = false;
const pausing = boundary.pause().then(result => { assert(result); paused = true; });
assert.equal(writer.snapshot().state, QueueState.pausing);
assert(consumer.process(output, 1152)); assert(output[0].every(value => value === 0));
assert.equal(writer.snapshot().state, QueueState.paused);
assert.equal(writer.snapshot().copiedFrames, 128);
await delay(8); assert.equal(paused, false); assert.deepEqual(calls, []);
deviceFrame = 1152; monitor.sample(); await pausing;
assert.deepEqual(calls, ['suspend']); assert.equal(monitor.frames, 128);
// Paused callbacks never consume queued frames or add starvation diagnostics.
const underruns = writer.snapshot().underrunFrames;
for (let i = 0; i < 20; i++) { consumer.process(output, 1280 + i * 128); assert(output[0].every(value => value === 0)); }
assert.equal(writer.snapshot().copiedFrames, 128); assert.equal(writer.snapshot().underrunFrames, underruns);
assert(writer.write(new Float32Array(128).fill(.75))); // bounded worker prefill allowed
assert.equal(await boundary.pause(), true); assert.deepEqual(calls, ['suspend']);
const resumed = boundary.resume(); assert.deepEqual(calls, ['suspend', 'resume']); // user-gesture call before await
await resumed; assert.equal(writer.snapshot().state, QueueState.running);
consumer.process(output, 4096);
for (let i = 0; i < 128; i++) assert.equal(output[0][i], (i + 129) / 1024);
assert.equal(monitor.reader.observe({contextTime: 4000 / 48000, performanceTime: 1}), 128); // pause gap excluded
// Pause/resume more than once; same PCM ordinal domain and no reset/duplicates.
const secondPause = boundary.pause(); consumer.process(output, 4224); await delay(8);
deviceFrame = 4224; monitor.sample(); await secondPause;
assert.equal(monitor.frames, 256); assert.deepEqual(calls, ['suspend', 'resume', 'suspend']);
await boundary.resume(); writer.sealEOF(); consumer.process(output, 4352);
assert(output[0].every(value => value === .75)); assert.equal(writer.snapshot().state, QueueState.done);
assert.equal(await boundary.pause(), false); // completion wins a pause race
const tail = monitor.finish(384); deviceFrame = 4480; monitor.sample(); await tail;
assert.equal(monitor.frames, 384); boundary.close(); monitor.close();

// Cancellation interrupts a pending gate wait promptly and closes queued output.
const closingQueue = createOutputQueue(1), closingMonitor = new PresentationMonitor(context, createPresentationTimeline(), () => {});
const closing = new OutputBoundary(context, closingQueue, closingMonitor);
const wait = closing.pause(); closing.close(); await assert.rejects(wait, /cancelled/);
assert.equal(new OutputWriter(closingQueue).snapshot().state, QueueState.closed); closingMonitor.close();
console.log(JSON.stringify({ suspendAfterCallbackAndPresentationBoundary: true, pausedCallbacksConsumeNoPCM: true, retainedPCMOrderOnResume: true, repeatedPauseResume: true, pauseGapExcluded: true, gestureResumeBeforeAwait: true, completionWinsPauseRace: true, cancelPendingGate: true, syntheticDeviceClock: true }, null, 2));
