import assert from 'node:assert/strict';
import { createPresentationTimeline, PresentationReader, PresentationWriter, TimelineError } from '../web/public/practice/presentation-timeline.mjs';
import { createOutputQueue, OutputWriter, OutputConsumer, QueueState } from '../web/public/practice/output-queue.mjs';
const rate = 48000;
const stamp = frame => ({ contextTime: (frame + .125) / rate, performanceTime: 123 });
const timeline = createPresentationTimeline(8), reader = new PresentationReader(timeline, rate);
const queue = createOutputQueue(1, 128), writer = new OutputWriter(queue), consumer = new OutputConsumer(queue, timeline);
const out = [new Float32Array(128)];
writer.start();
// Silence has no PCM ordinal. Primed/underrun context frames cannot count audio.
consumer.process(out, 0); assert.equal(reader.observe(stamp(128)), 0);
writer.write(new Float32Array(64).fill(.25)); consumer.process(out, 256);
assert.equal(writer.snapshot().copiedFrames, 64);
assert.equal(reader.observe(stamp(256)), 0); // currently-rendered sample excluded
assert.equal(reader.observe(stamp(287)), 31);
assert.equal(reader.observe(stamp(287)), 31); // duplicate timestamp
assert.equal(reader.observe(stamp(320)), 64);
consumer.process(out, 384); // a full underrun gap
writer.write(new Float32Array(128).fill(.5)); consumer.process(out, 512);
assert.equal(reader.observe(stamp(500)), 64);
assert.equal(reader.observe(stamp(540)), 92);
assert.equal(reader.observe(stamp(640)), 192);
assert.equal(reader.observe(stamp(1000)), 192); // idle/device time cannot add PCM
assert.equal(writer.snapshot().underrunFrames, 320);
assert.equal(reader.observe(stamp(999)), 192); // stale clock estimate adds nothing
assert.throws(() => reader.observe({contextTime: NaN, performanceTime: 1}), TimelineError);
assert.equal(consumer.process(out, 1024), true); // empty queue remains silent
writer.write(new Float32Array(128));
assert.equal(consumer.process(out, 1152), false); // clock failure stops next audio
assert.equal(writer.snapshot().state, QueueState.error);
assert.equal(writer.snapshot().copiedFrames, 192);
assert(out[0].every(value => value === 0));

// Exhaustion never overwrites unacknowledged entries or copies an untracked block.
const small = createPresentationTimeline(2), smallQueue = createOutputQueue(1, 128);
const sw = new OutputWriter(smallQueue), sc = new OutputConsumer(smallQueue, small);
sw.start();
for (let i = 0; i < 2; i++) { sw.write(new Float32Array(128).fill(.75)); assert(sc.process(out, i * 128)); }
sw.write(new Float32Array(128).fill(.75)); assert.equal(sc.process(out, 256), false);
assert.equal(sw.snapshot().copiedFrames, 256); assert(out[0].every(value => value === 0));
assert.throws(() => new PresentationReader(small, rate).observe(stamp(1000)), TimelineError);

// Released slots are reusable; both ring indices and 64-bit context/PCM values wrap.
const wrap = createPresentationTimeline(2), ww = new PresentationWriter(wrap), wr = new PresentationReader(wrap, rate);
const header = new Int32Array(wrap); header[2] = -1; header[3] = -1;
const origin = 3 * 2 ** 32 - 32;
ww.totalFrames = origin; wr.completedFrames = origin; wr.presentedFrames = origin;
assert(ww.canAppend(origin, 64)); ww.append(origin, 64);
assert.equal(wr.observe(stamp(origin + 64)), origin + 64);
assert(ww.canAppend(origin + 128, 64)); ww.append(origin + 128, 64);
assert.equal(wr.observe(stamp(origin + 192)), origin + 128);
assert.equal(header[2], 1); assert.equal(header[3], 1);
for (const value of [NaN, Infinity, -1, Number.MAX_SAFE_INTEGER]) {
 const w = new PresentationWriter(createPresentationTimeline(2));
 assert.equal(w.canAppend(value, 128), false);
}
for (const timestamp of [{contextTime: -1, performanceTime: 1}, {contextTime: 1, performanceTime: NaN}, {contextTime: Infinity, performanceTime: 1}, {}]) {
 assert.throws(() => new PresentationReader(createPresentationTimeline(2), rate).observe(timestamp), TimelineError);
}
const corrupt = createPresentationTimeline(2), cw = new PresentationWriter(corrupt);
assert(cw.canAppend(0, 128)); cw.append(0, 128); new Int32Array(corrupt)[10] = 8193;
assert.throws(() => new PresentationReader(corrupt, rate).observe(stamp(128)), TimelineError);
assert.throws(() => createPresentationTimeline(3), TimelineError);
assert.throws(() => new PresentationReader(createPresentationTimeline(), 1), TimelineError);

// Actual production worklet class, in another Node thread. This tests atomic
// publication/retirement across threads; timestamps are synthetic, not hardware.
const { Worker } = await import('node:worker_threads');
const shared = createPresentationTimeline(1024), sharedReader = new PresentationReader(shared, rate);
const worker = new Worker(new URL('./browser-presentation-node-worker.mjs', import.meta.url), { workerData: { timeline: shared } });
let total = 0, blocks = 0;
const finished = new Promise((resolve, reject) => {
 worker.on('error', reject);
 worker.on('exit', code => { if (code) reject(new Error('Presentation worker exited: ' + code)); });
 worker.on('message', message => {
  try {
   if (message.error) throw new Error(message.error);
   if (message.done) { resolve(); return; }
   assert.equal(sharedReader.observe(stamp(message.contextEnd)), message.copiedFrames);
   assert.equal(sharedReader.observe(stamp(message.contextEnd)), message.copiedFrames);
   total = message.copiedFrames; blocks += message.blocks;
   worker.postMessage('retired');
  } catch (error) { reject(error); }
 });
});
const deadline = setTimeout(() => worker.terminate(), 15000);
try { await finished; assert.equal(blocks, 8192); assert.equal(total, 786432); }
finally { clearTimeout(deadline); await worker.terminate(); }
console.log(JSON.stringify({ contextDeviceMapping: true, underrunGapsExcluded: true, partialAndDuplicateTimestamps: true, overflowStopsBeforeCopy: true, uint32CursorWrap: true, uint64Coordinates: true, concurrentWorkletBlocks: blocks, concurrentCopiedFrames: total, physicalDeviceEvidence: false }, null, 2));

// Completion waits for device position, even when the copied report is already
// complete. Closing/failing the monitor rejects its wait and releases its timer.
const { PresentationMonitor } = await import('../web/public/practice/presentation-monitor.mjs');
const mt = createPresentationTimeline(4), mw = new PresentationWriter(mt);
assert(mw.canAppend(128, 128)); mw.append(128, 128);
let deviceFrame = 128, failure;
const monitor = new PresentationMonitor({ sampleRate: rate, getOutputTimestamp: () => stamp(deviceFrame) }, mt, error => { failure = error; });
let done = false;
const completion = monitor.finish(128).then(() => { done = true; });
await Promise.resolve(); assert.equal(done, false);
deviceFrame = 200; monitor.sample(); await Promise.resolve(); assert.equal(done, false);
deviceFrame = 256; monitor.sample(); await completion; assert.equal(done, true); assert.equal(failure, undefined); monitor.close(); monitor.close();
const closing = new PresentationMonitor({ sampleRate: rate, getOutputTimestamp: () => stamp(0) }, createPresentationTimeline(), () => {});
const waiting = closing.finish(128); closing.close(); await assert.rejects(waiting, /closed/);
const failing = new PresentationMonitor({ sampleRate: rate, getOutputTimestamp: () => stamp(512) }, createPresentationTimeline(), error => { failure = error; });
failing.sample(); failing.context.getOutputTimestamp = () => ({contextTime: NaN, performanceTime: 1});
const invalidClock = failing.finish(128); await assert.rejects(invalidClock, /Invalid output timestamp/); assert(failure instanceof TimelineError); failing.close();
console.log(JSON.stringify({ monitorWaitsForDevicePosition: true, monitorCloseRejectsWait: true, monitorClockFailureRejectsWait: true }));

// Repeated host pulls for the identical render window are idempotent. Reuse the
// original stereo PCM, even after its timeline entry was retired. Pause waits
// for a distinct quantum; an EOF tail is reproducible without another count.
const rq=createOutputQueue(2,512), rw=new OutputWriter(rq), rt=createPresentationTimeline(8), rc=new OutputConsumer(rq,rt), rr=new PresentationReader(rt,rate);
const interleaved=new Float32Array(512);
for(let i=0;i<256;i++){interleaved[i*2]=i/512;interleaved[i*2+1]=-i/512;}
rw.write(interleaved);rw.start();
const firstBlock=[new Float32Array(128),new Float32Array(128)];rc.process(firstBlock,441216);
assert.equal(rr.observe(stamp(441344)),128);
const beforeRepeat=rw.snapshot();rw.requestPause();
const duplicate=[new Float32Array(128),new Float32Array(128)];
for(let i=0;i<4;i++){assert.equal(rc.process(duplicate,441216),true);assert.deepEqual(duplicate,firstBlock);}
assert.equal(rw.snapshot().copiedFrames,beforeRepeat.copiedFrames);assert.equal(rw.snapshot().underrunFrames,beforeRepeat.underrunFrames);assert.equal(rw.snapshot().state,QueueState.pausing);
assert.equal(rc.process(duplicate,441344),true);assert(duplicate.every(channel=>channel.every(value=>value===0)));assert.equal(rw.snapshot().state,QueueState.paused);
rw.resume();rw.sealEOF();assert.equal(rc.process(duplicate,441472),false);const terminal=duplicate.map(channel=>channel.slice());
assert.equal(rc.process(duplicate,441472),false);assert.deepEqual(duplicate,terminal);assert.equal(rw.snapshot().copiedFrames,256);assert.equal(rr.observe(stamp(441600)),256);
const badq=createOutputQueue(1,512), badw=new OutputWriter(badq), badc=new OutputConsumer(badq,createPresentationTimeline());badw.write(new Float32Array(256));badw.start();
assert(badc.process(out,1024));assert.equal(badc.process(out,1088),false);assert.match(badw.failureMessage(),/Invalid presentation coordinates/);assert.equal(badw.snapshot().copiedFrames,128);assert(out[0].every(value=>value===0));
console.log(JSON.stringify({duplicateRenderWindowReusesPCM:true,noDuplicateCopiesOrAcknowledgements:true,pauseRequiresNewRenderBoundary:true,duplicateEOFTailPreserved:true,otherOverlapsFailClosed:true}));
