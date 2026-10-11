// Lifecycle tests for the actual presentation facade. Device/worker mocks attest
// cancellation and ownership only, not DSP or browser audio presentation.
import assert from 'node:assert/strict';
import { setImmediate as tick } from 'node:timers/promises';
class MockWorker {
 static instances = [];
 constructor() { this.sent = []; this.terminations = 0; MockWorker.instances.push(this); }
 postMessage(message) { this.sent.push(message); }
 terminate() { this.terminations++; }
 emit(data) { this.onmessage?.({ data }); }
}
class MockAudioContext {
 static instances = [];
 constructor({ sampleRate }) { this.sampleRate = sampleRate; this.state = 'suspended'; this.closeCalls = 0; this.resumeCalls = 0; this.baseLatency = 0; this.outputLatency = 0; this.destination = {}; MockAudioContext.instances.push(this); }
 resume() { this.resumeCalls++; this.state = 'running'; return Promise.resolve(); }
 suspend() { this.state = 'suspended'; return Promise.resolve(); }
 close() { this.closeCalls++; this.state = 'closed'; return Promise.resolve(); }
}
MockAudioContext.prototype.audioWorklet = { addModule: () => Promise.resolve() };
class MockAudioNode { connect() {} disconnect() {} }
globalThis.Worker = MockWorker;
globalThis.AudioContext = MockAudioContext;
globalThis.AudioWorkletNode = MockAudioNode;
globalThis.crossOriginIsolated = true;
const { BrowserPractice } = await import('../web/src/practice.ts');
const file = new File(['fixture'], 'fixture.wav');
let client = new BrowserPractice();
const opening = client.open(file);
client.dispose(); client.dispose();
await assert.rejects(opening, /closed/);
assert.equal(MockWorker.instances.at(-1).terminations, 1);
assert.equal(MockWorker.instances.at(-1).sent.length, 0);

client = new BrowserPractice();
let worker = MockWorker.instances.at(-1); worker.emit({ type: 'ready' });
const pending = client.open(file); await tick();
assert.equal(worker.sent.length, 1);
client.dispose(); await assert.rejects(pending, /closed/);

const snapshot = { project: { track: { audio: { sampleRate: 48000, channels: 1 } } }, capabilities: { streamPreview: true } };
client = new BrowserPractice({ loadOutput: () => new Promise(() => {}) });
worker = MockWorker.instances.at(-1); worker.emit({ type: 'ready' });
const preflight = client.preview(snapshot);
const context = MockAudioContext.instances.at(-1);
assert.equal(context.resumeCalls, 1); // resume occurred synchronously from click
client.cancel(); await assert.rejects(preflight, /cancelled/);
assert.equal(context.closeCalls, 1);
assert(!worker.sent.some(message => message.request?.type === 'preview'));
client.dispose(); assert.equal(context.closeCalls, 1);

client = new BrowserPractice({ loadOutput: () => Promise.resolve({ createOutputQueue: () => new SharedArrayBuffer(1) }) });
worker = MockWorker.instances.at(-1); worker.emit({ type: 'ready' });
const active = client.preview(snapshot); await tick();
const activeContext = MockAudioContext.instances.at(-1);
assert(worker.sent.some(message => message.request?.type === 'preview'));
client.dispose(); await assert.rejects(active, /closed/);
assert.equal(activeContext.closeCalls, 1); // dispose/finally share one close fence

client = new BrowserPractice({ loadOutput: () => new Promise(() => {}) });
worker = MockWorker.instances.at(-1); worker.emit({ type: 'ready' });
const failedSetup = client.preview(snapshot);
worker.emit({ type: 'fatal', message: 'worker failed' });
await assert.rejects(failedSetup, /worker failed/);
assert.equal(MockAudioContext.instances.at(-1).closeCalls, 1);
assert.equal(worker.terminations, 1);
console.log(JSON.stringify({ closedBeforeReady: true, pendingRequestsRejected: true, gestureResumeBeforeAwait: true, cancelDuringSetup: true, audioCloseExactlyOnce: true, fatalDuringSetup: true }, null, 2));

// Actual monitor/timeline modules with device timestamps controlled by this
// lifecycle fixture. Core reply is a teardown fence, not the presentation clock.
const { createOutputQueue, OutputConsumer, OutputWriter } = await import('../web/public/practice/output-queue.mjs');
const { createPresentationTimeline } = await import('../web/public/practice/presentation-timeline.mjs');
const { PresentationMonitor } = await import('../web/public/practice/presentation-monitor.mjs');
const queueModule = { createOutputQueue, createPresentationTimeline, PresentationMonitor };
MockAudioContext.prototype.getOutputTimestamp = function() { return { contextTime: this.deviceTime || 0, performanceTime: 123 }; };
let capturedNode;
globalThis.AudioWorkletNode = class extends MockAudioNode { constructor(_context, _name, options) { super(); capturedNode = options.processorOptions; } };
client = new BrowserPractice({ loadOutput: () => Promise.resolve(queueModule) });
worker = MockWorker.instances.at(-1); worker.emit({ type: 'ready' });
let resolved = false;
const presented = client.preview(snapshot).then(result => { resolved = true; return result; }); await tick();
const timestampContext = MockAudioContext.instances.at(-1);
const outputWriter = new OutputWriter(capturedNode.queue), outputConsumer = new OutputConsumer(capturedNode.queue, capturedNode.timeline);
outputWriter.write(new Float32Array(128)); outputWriter.start(); outputWriter.sealEOF(); outputConsumer.process([new Float32Array(128)], 128);
const request = worker.sent.find(message => message.request?.type === 'preview');
worker.emit({ type: 'result', id: request.id, response: { version: 1, preview: { outputFrames: 128, copiedFrames: 128 } } });
await tick(); assert.equal(resolved, false); assert.equal(timestampContext.closeCalls, 0);
timestampContext.deviceTime = 256 / 48000; await presented;
assert.equal(timestampContext.closeCalls, 1); client.dispose();

// Cancel while waiting for presentation releases the monitor and context.
client = new BrowserPractice({ loadOutput: () => Promise.resolve(queueModule) });
worker = MockWorker.instances.at(-1); worker.emit({ type: 'ready' });
const cancelDrain = client.preview(snapshot); await tick();
const drainContext = MockAudioContext.instances.at(-1);
const drainRequest = worker.sent.find(message => message.request?.type === 'preview');
worker.emit({ type: 'result', id: drainRequest.id, response: { version: 1, preview: { outputFrames: 128, copiedFrames: 128 } } });
await tick(); client.cancel(); await assert.rejects(cancelDrain, /cancelled/); assert.equal(drainContext.closeCalls, 1); client.dispose();
console.log(JSON.stringify({ callbackReportDoesNotFinishPresentation: true, deviceTimestampFinishesPreview: true, cancelDuringPresentationClosesMonitor: true }));

// Clock failure cancels output but still awaits the Go reader teardown reply.
let ownedMonitor;
class CapturedMonitor extends PresentationMonitor { constructor(...args) { super(...args); ownedMonitor = this; } }
client = new BrowserPractice({ loadOutput: () => Promise.resolve({ ...queueModule, PresentationMonitor: CapturedMonitor }) });
worker = MockWorker.instances.at(-1); worker.emit({ type: 'ready' });
const clockFailure = client.preview(snapshot); void clockFailure.catch(() => {}); await tick();
const failedClockContext = MockAudioContext.instances.at(-1);
failedClockContext.getOutputTimestamp = () => ({contextTime: NaN, performanceTime: 1});
ownedMonitor.sample(); await tick();
assert.equal(failedClockContext.closeCalls, 0);
assert(worker.sent.some(message => message.type === 'cancel'));
const failedClockRequest = worker.sent.find(message => message.request?.type === 'preview');
worker.emit({ type: 'result', id: failedClockRequest.id, response: { version: 1, error: {code: 'cancelled', message: 'context canceled'} } });
await assert.rejects(clockFailure, /Invalid output timestamp/);
assert.equal(failedClockContext.closeCalls, 1); client.dispose();
console.log(JSON.stringify({ clockFailureAwaitsCoreTeardownFence: true }));
