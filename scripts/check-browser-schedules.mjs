import assert from 'node:assert/strict';
import { ScheduleAcknowledgements } from '../web/public/practice/schedule-acknowledgements.mjs';
import { createPresentedCounter, PresentedWriter, PresentedReader, PresentedBusyError, PresentedState } from '../web/public/practice/presented-counter.mjs';
const ledger = new ScheduleAcknowledgements();
const generation = '18446744073709551614'; ledger.reset(generation);
const first = {generation, sequence: '9007199254740993', outputStart: 0, outputEnd: 100};
ledger.accept(first); first.outputEnd = 1; // accepted schedule is immutable
ledger.accept({generation, sequence: '9007199254740994', outputStart: 100, outputEnd: 250});
assert.throws(() => ledger.accept({generation, sequence: '9007199254740995', outputStart: 250, outputEnd: 350}), /full/);
assert.deepEqual(ledger.drain(0, 128), []);
assert.deepEqual(ledger.drain(50, 128), [{generation, sequence: '9007199254740993', consumedOutputFrame: 50}]);
assert.deepEqual(ledger.drain(50, 128), []);
assert.deepEqual(ledger.drain(125, 1), [{generation, sequence: '9007199254740993', consumedOutputFrame: 100}]);
assert.deepEqual(ledger.drain(125, 1), [{generation, sequence: '9007199254740994', consumedOutputFrame: 125}]);
ledger.accept({generation, sequence: '9007199254740995', outputStart: 250, outputEnd: 350});
assert.deepEqual(ledger.drain(350, 128), [{generation, sequence: '9007199254740994', consumedOutputFrame: 250}, {generation, sequence: '9007199254740995', consumedOutputFrame: 350}]);
assert.throws(() => ledger.drain(349, 1)); assert.throws(() => ledger.drain(351, 1)); assert.throws(() => ledger.drain(350, 129));
ledger.reset('18446744073709551615'); assert.deepEqual(ledger.drain(0, 128), []);
assert.throws(() => ledger.accept({generation, sequence: '9007199254740996', outputStart: 0, outputEnd: 1}));
for (const id of [1, '', '01', '-1', '18446744073709551616']) assert.throws(() => ledger.reset(id));
assert.throws(() => ledger.reset('18446744073709551615'));
ledger.accept({generation: '18446744073709551615', sequence: '18446744073709551615', outputStart: 0, outputEnd: 1});
assert.deepEqual(ledger.drain(1, 128), [{generation: '18446744073709551615', sequence: '18446744073709551615', consumedOutputFrame: 1}]);
const shared = createPresentedCounter(), writer = new PresentedWriter(shared), reader = new PresentedReader(shared);
assert.deepEqual(reader.snapshot(), {frames: 0, state: PresentedState.active});
writer.publish(3 * 2 ** 32 + 17); assert.equal(reader.snapshot().frames, 3 * 2 ** 32 + 17);
writer.publish(Number.MAX_SAFE_INTEGER, PresentedState.quiescent); assert.deepEqual(reader.snapshot(), {frames: Number.MAX_SAFE_INTEGER, state: PresentedState.quiescent});
assert.throws(() => writer.publish(Number.MAX_SAFE_INTEGER + 1)); assert.throws(() => writer.publish(1));
new Int32Array(shared)[1] |= 1; assert.throws(() => reader.snapshot(), PresentedBusyError);
const routingEvidence = { boundedSchedules: 2, orderedPartialAndCompleteAcknowledgements: true, boundedDrainPreservesUndeliveredEntries: true, duplicatePresentationAddsNoAck: true, staleGenerationRejected: true, exactUint64Identifiers: true, safeDeviceFrameCounter: true, physicalClockEvidence: false };
// A live prefix keeps its old ID, drops stale lookahead and leaves two traversal
// slots. Presented prefixes may never be rewritten by a subsequent edit.
const edited=new ScheduleAcknowledgements();edited.reset('1');
edited.accept({generation:'1',sequence:'1',outputStart:0,outputEnd:1000});
edited.accept({generation:'1',sequence:'2',outputStart:1000,outputEnd:2000});
edited.drain(100,128);
assert.throws(()=>edited.replaceTail('1',100,{generation:'1',sequence:'3',outputStart:100,outputEnd:2000}));
assert.throws(()=>edited.replaceTail('1',400,{generation:'2',sequence:'3',outputStart:400,outputEnd:2000}));
edited.replaceTail('1',400,{generation:'1',sequence:'3',outputStart:400,outputEnd:1600});
edited.accept({generation:'1',sequence:'4',outputStart:1600,outputEnd:3200});
assert.throws(()=>edited.accept({generation:'1',sequence:'5',outputStart:3200,outputEnd:4000}),/full/);
assert.deepEqual(edited.drain(1700,128),[{generation:'1',sequence:'1',consumedOutputFrame:400},{generation:'1',sequence:'3',consumedOutputFrame:1600},{generation:'1',sequence:'4',consumedOutputFrame:1700}]);

// Production C++ selection WASM, scheduler, worklet copying and presentation
// mapping. This is a Node platform pump with synthetic device timestamps.
const createModule = (await import('../web/public/practice/dsp.mjs')).default;
const { ScheduledSelections } = await import('../web/public/practice/scheduled-selections.mjs');
const { createOutputQueue, OutputWriter } = await import('../web/public/practice/output-queue.mjs');
const { createPresentationTimeline, PresentationReader } = await import('../web/public/practice/presentation-timeline.mjs');
const { setTimeout: delay } = await import('node:timers/promises');
let Processor;
globalThis.AudioWorkletProcessor = class {};
globalThis.registerProcessor = (_name, processor) => { Processor = processor; };
await import('../web/public/practice/output-worklet.mjs');
const module = await createModule();
const source = {source: 'verified-resource', channels: 1, sampleRate: 48000, frames: 48000};
let reads = 0, maxRead = 0, notified;
const processing = new ScheduledSelections(module, source, async (start, count) => {
 assert(start >= 12000 && start + count <= 36000 && count <= 512);
 reads++; maxRead = Math.max(maxRead, count);
 const pcm = new Float32Array(count);
 for (let i = 0; i < count; i++) pcm[i] = .2 * Math.cos(2 * Math.PI * 440 * (start + i) / 48000);
 return pcm;
}, error => { notified = error; });
const schedule = (generation, sequence, outputStart) => ({generation, sequence, source: source.source, startFrame: 12000, endFrame: 36000, outputStart, outputEnd: outputStart + 32000, speed: .75, pitch: 3, edgeFadeFrames: 240});
const device = createPresentedCounter(), deviceWriter = new PresentedWriter(device);
const pcmQueue = createOutputQueue(1), pcmWriter = new OutputWriter(pcmQueue), mapping = createPresentationTimeline(), mapper = new PresentationReader(mapping, 48000);
const processor = new Processor({processorOptions: {queue: pcmQueue, timeline: mapping}});
await processing.reset('1', pcmQueue, device);
processing.schedule(schedule('1', '1', 0)); processing.schedule(schedule('1', '2', 32000));
assert.throws(() => processing.schedule(schedule('1', '3', 64000)), /full/);
await processing.prefilled(); pcmWriter.start();
const channel = new Float32Array(128), outputs = [[channel]];
let contextFrame = 0; const completions = [], partials = [];
const deadline = performance.now() + 10000;
while (pcmWriter.snapshot().copiedFrames < 64000) {
 assert(performance.now() < deadline); globalThis.currentFrame = contextFrame;
 processor.process([], outputs); contextFrame += 128;
 for (const sample of channel) assert(Number.isFinite(sample));
 const presented = mapper.observe({contextTime: Math.max(0, contextFrame - 256) / 48000, performanceTime: 1});
 deviceWriter.publish(presented);
 for (const ack of processing.drain(128)) { partials.push(ack); if (ack.consumedOutputFrame === Number(ack.sequence) * 32000) completions.push(ack); }
 await delay(1);
}
assert.equal(completions.length, 1); // copied tail isn't yet presented
const presented = mapper.observe({contextTime: (contextFrame + 256) / 48000, performanceTime: 1});
deviceWriter.publish(presented, PresentedState.quiescent);
completions.push(...processing.drain(128));
assert.deepEqual(completions, [{generation:'1', sequence:'1', consumedOutputFrame:32000}, {generation:'1', sequence:'2', consumedOutputFrame:64000}]);
assert(partials.some(ack => ack.consumedOutputFrame > 0 && ack.consumedOutputFrame < 32000));
assert.deepEqual(processing.drain(128), []); assert.equal(notified, undefined); assert.equal(maxRead, 512);
// A generation reset drops queued DSP work and old device acknowledgements.
const freshQueue = createOutputQueue(1), freshDevice = createPresentedCounter();
await processing.reset('2', freshQueue, freshDevice);
assert.deepEqual(processing.drain(128), []); assert.throws(() => processing.schedule(schedule('1', '3', 0)));
processing.schedule(schedule('2', '3', 0)); await processing.prefilled(); await processing.close();
assert.equal(notified, undefined); assert.throws(() => processing.drain(128), /closed/);

// Failure cannot escape drain until the caller attests device quiescence. Close
// also fences an in-flight source read before releasing processing ownership.
const failingQueue = createOutputQueue(1), failingDevice = createPresentedCounter(), failedWriter = new PresentedWriter(failingDevice);
let rejectRead, failureNotified;
const blockedRead = new Promise((_resolve, reject) => { rejectRead = reject; });
const failing = new ScheduledSelections(module, source, () => blockedRead, error => { failureNotified = error; });
await failing.reset('1', failingQueue, failingDevice); failing.schedule(schedule('1', '1', 0));
rejectRead(new Error('source read failed')); await delay(1);
assert(failureNotified); assert.deepEqual(failing.drain(128), []);
failedWriter.publish(0, PresentedState.failed); assert.throws(() => failing.drain(128), /source read failed/); await failing.close();
let resolveRead, closed = false;
const readFence = new Promise(resolve => { resolveRead = resolve; });
const closing = new ScheduledSelections(module, source, () => readFence, error => { throw error; });
await closing.reset('1', createOutputQueue(1), createPresentedCounter()); closing.schedule(schedule('1', '1', 0));
const closeFence = closing.close().then(() => { closed = true; });
let secondClosed = false; const secondCloseFence = closing.close().then(() => { secondClosed = true; });
await delay(1); assert.equal(closed, false); assert.equal(secondClosed, false);
resolveRead(new Float32Array(512)); await closeFence; await secondCloseFence; assert.equal(closed, true); assert.equal(secondClosed, true);
console.log(JSON.stringify({...routingEvidence, actualSelectionWASM: true, scheduledOutputFrames:64000, completedScheduleAcknowledgements:completions, boundedReadFrames:maxRead, sourceReads:reads, copiedTailNotAcknowledged: true, generationReset: true, failureWaitsForHostQuiescence: true, closeFencesSourceRead: true, deviceClockIsSynthetic: true}, null, 2));
