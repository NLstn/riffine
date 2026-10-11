import assert from 'node:assert/strict';
import { setTimeout as delay } from 'node:timers/promises';
import { LiveOutputHost } from '../web/public/practice/live-output-host.mjs';
import { OutputWriter } from '../web/public/practice/output-queue.mjs';
import { PresentedReader, PresentedState } from '../web/public/practice/presented-counter.mjs';
let Processor;
globalThis.AudioWorkletProcessor = class {};
globalThis.registerProcessor = (_name, processor) => { Processor = processor; };
await import('../web/public/practice/output-worklet.mjs');
class Node {
 constructor(_context, _name, options) { this.processor = new Processor(options); this.disconnects = 0; }
 connect() {} disconnect() { this.disconnects++; }
}
globalThis.AudioWorkletNode = Node;
class Context {
 constructor() { this.sampleRate = 48000; this.state = 'running'; this.frame = 0; this.suspends = 0; this.closes = 0; }
 getOutputTimestamp() { return {contextTime: (this.frame + .125) / this.sampleRate, performanceTime: 1}; }
 async suspend() { this.suspends++; this.state = 'suspended'; }
 async resume() { this.state = 'running'; }
 async close() { this.closes++; this.state = 'closed'; }
}
const context = new Context(); let notified;
const host = new LiveOutputHost(context, 1, error => { notified = error; });
const first = await host.reset(), reader = new PresentedReader(first.presented), writer = new OutputWriter(first.queue);
assert.equal(context.state, 'suspended'); assert.deepEqual(reader.snapshot(), {frames:0, state:PresentedState.quiescent});
writer.write(new Float32Array(128).fill(.25)); await host.start();
const output = [[new Float32Array(128)]]; globalThis.currentFrame = 1024; host.node.processor.process([], output);
host.monitor.sample(); assert.deepEqual(reader.snapshot(), {frames:0, state:PresentedState.active});
const pausing = host.suspend(); globalThis.currentFrame = 1152; host.node.processor.process([], output);
await delay(8); assert.equal(context.suspends, 1); // reset only; copied is not presented
context.frame = 1152; host.monitor.sample(); await pausing;
assert.equal(context.suspends, 2); assert.deepEqual(reader.snapshot(), {frames:128, state:PresentedState.quiescent});
const oldNode = host.node; writer.close(); // worker resource fence after suspension
const second = await host.reset(); assert.notEqual(second.queue, first.queue); assert.notEqual(second.presented, first.presented);
assert.equal(oldNode.disconnects, 1); assert.deepEqual(new PresentedReader(second.presented).snapshot(), {frames:0,state:PresentedState.quiescent});
await host.close(); await host.close(); assert.equal(context.closes, 1); assert.equal(notified, undefined);

// A failing device is published to the worker only after context-close resolves.
const failingContext = new Context(); let finishClose;
failingContext.close = () => { failingContext.closes++; return new Promise(resolve => { finishClose = () => { failingContext.state='closed'; resolve(); }; }); };
let failures = 0;
const failingHost = new LiveOutputHost(failingContext, 1, () => { failures++; });
const failingBuffers = await failingHost.reset(), failingReader = new PresentedReader(failingBuffers.presented);
const failed = failingHost.fail(new Error('device fault')); await delay(1);
assert.equal(failingReader.snapshot().state, PresentedState.quiescent); assert.equal(failures, 0);
finishClose(); await failed; assert.equal(failingReader.snapshot().state, PresentedState.failed); assert.equal(failures, 1);
await assert.rejects(failingHost.start(), /device fault/); await failingHost.close(); assert.equal(failingContext.closes, 1);
console.log(JSON.stringify({ mainHostPublishesDevicePosition: true, suspendWaitsForPresentation: true, resetUsesFreshEpochBuffers: true, failurePublicationAfterCloseFence: true, contextClosesExactlyOnce: true, physicalDeviceEvidence:false }, null, 2));

const stalledContext = new Context(), stalledHost = new LiveOutputHost(stalledContext, 1, () => {});
const stalledBuffers = await stalledHost.reset(), stalledReader = new PresentedReader(stalledBuffers.presented);
new OutputWriter(stalledBuffers.queue).write(new Float32Array(128)); await stalledHost.start();
globalThis.currentFrame = 128; stalledHost.node.processor.process([], output);
stalledHost.monitor.progressAt = performance.now() - 10001; stalledHost.monitor.sample();
await stalledHost.closing; assert.equal(stalledReader.snapshot().state, PresentedState.failed);
assert.match(stalledHost.failure.message, /stopped advancing/); await stalledHost.close();
console.log(JSON.stringify({ stalledPresentationClosesBeforeFailedPublication:true }));

// Close rejection falls back to a real suspension fence, never immediate failure
// publication. If both fences reject, the shared counter stays unacknowledged.
const rejectedContext = new Context(); let suspendFence;
const rejectedHost = new LiveOutputHost(rejectedContext, 1, () => {});
const rejectedBuffers = await rejectedHost.reset(), rejectedReader = new PresentedReader(rejectedBuffers.presented);
rejectedContext.close = async () => { rejectedContext.closes++; throw new Error('close rejected'); };
rejectedContext.suspend = () => new Promise(resolve => { suspendFence = () => { rejectedContext.state='suspended'; resolve(); }; });
const fallback = rejectedHost.fail(new Error('output failed')); await delay(1);
assert.equal(rejectedReader.snapshot().state, PresentedState.quiescent);
suspendFence(); await fallback; assert.equal(rejectedReader.snapshot().state, PresentedState.failed);
await rejectedHost.close(); assert.equal(rejectedContext.closes, 1);
const unavailableContext = new Context(), unavailableHost = new LiveOutputHost(unavailableContext, 1, () => {});
const unavailableBuffers = await unavailableHost.reset(), unavailableReader = new PresentedReader(unavailableBuffers.presented);
unavailableContext.close = async () => { throw new Error('close rejected'); };
unavailableContext.suspend = async () => { throw new Error('suspend rejected'); };
await assert.rejects(unavailableHost.fail(new Error('device fault')), /could not quiesce/);
assert.equal(unavailableReader.snapshot().state, PresentedState.quiescent);
console.log(JSON.stringify({ closeRejectionWaitsForSuspendFence:true, failedFencesDoNotPublishQuiescence:true }));
