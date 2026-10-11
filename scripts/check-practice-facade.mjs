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

// Pause cancellation retains the pending core reader fence, then closes once.
const { OutputBoundary } = await import('../web/public/practice/output-boundary.mjs');
let controlsReady = 0;
client = new BrowserPractice({ loadOutput: () => Promise.resolve({ ...queueModule, OutputBoundary }) });
worker = MockWorker.instances.at(-1); worker.emit({ type: 'ready' });
const controlled = client.preview(snapshot, () => { controlsReady++; }); await tick();
assert.equal(controlsReady, 1);
const controlledContext = MockAudioContext.instances.at(-1);
const controlledWriter = new OutputWriter(capturedNode.queue), controlledConsumer = new OutputConsumer(capturedNode.queue, capturedNode.timeline);
controlledWriter.write(new Float32Array(256)); controlledWriter.start(); controlledConsumer.process([new Float32Array(128)], 128);
const pausingPreview = client.pausePreview(); controlledConsumer.process([new Float32Array(128)], 256);
controlledContext.deviceTime = 256 / 48000;
assert.equal(await pausingPreview, true); assert.equal(controlledContext.state, 'suspended');
const resumesBefore = controlledContext.resumeCalls;
const resumingPreview = client.resumePreview(); assert.equal(controlledContext.resumeCalls, resumesBefore + 1); await resumingPreview;
const secondPausing = client.pausePreview(); client.cancel(); await assert.rejects(secondPausing, /cancelled/);
assert.equal(controlledContext.closeCalls, 0); // worker still owns borrowed reader
const controlledRequest = worker.sent.find(message => message.request?.type === 'preview');
worker.emit({ type: 'result', id: controlledRequest.id, response: { version: 1, error: {code: 'cancelled', message: 'context canceled'} } });
assert.equal((await controlled).error.code, 'cancelled'); assert.equal(controlledContext.closeCalls, 1); client.dispose();
console.log(JSON.stringify({ previewControlsReady: true, facadePauseResume: true, cancelPauseRetainsReaderFence: true }));

// Live setup retains synchronous user activation. The actual host controller
// owns context close; a worker's attach/detach replies remain reader fences.
const { LiveOutputHost } = await import('../web/public/practice/live-output-host.mjs');
const liveSnapshot = {version:1,sequence:2,project:snapshot.project,capabilities:{streamPreview:true,liveTransport:false}};
client = new BrowserPractice({loadLive: () => new Promise(() => {})});
worker = MockWorker.instances.at(-1); worker.emit({type:'ready'});
const cancelledLiveSetup = client.transport(liveSnapshot,{type:'play'});
const cancelledLiveContext = MockAudioContext.instances.at(-1);
assert.equal(cancelledLiveContext.resumeCalls,1);
client.cancel(); await assert.rejects(cancelledLiveSetup,/cancelled/);
assert.equal(cancelledLiveContext.closeCalls,1);
assert(!worker.sent.some(message=>message.request?.type==='attach_transport'));
client.dispose();

client = new BrowserPractice({loadLive: async () => ({LiveOutputHost})});
worker = MockWorker.instances.at(-1); worker.emit({type:'ready'});
let hostID=0, requestsSeen=new Set();
async function hostRequest(method,args={}) {
 const id=++hostID; worker.emit({type:'live_call',id,method,args});
 for(let attempt=0;attempt<100;attempt++) {
  const response=worker.sent.find(message=>message.type==='live_result'&&message.id===id);
  if(response){assert.equal(response.error,undefined);return response.result;}
  await tick();
 }
 throw new Error('Host response missing: '+method);
}
async function nextRequest(type) {
 for(let attempt=0;attempt<100;attempt++) {
  const request=worker.sent.find(message=>message.request?.type===type&&!requestsSeen.has(message.id));
  if(request){requestsSeen.add(request.id);return request;}
  await tick();
 }
 throw new Error('Core request missing: '+type);
}
function coreReply(request,value) {worker.emit({type:'result',id:request.id,response:{version:1,snapshot:value}});}
const livePlay=client.transport(liveSnapshot,{type:'play'});
const originalLiveContext=MockAudioContext.instances.at(-1);
assert.equal(originalLiveContext.resumeCalls,1);
let attachRequest=await nextRequest('attach_transport');
await hostRequest('open',{sampleRate:48000,channels:1});await hostRequest('reset');
const attachedSnapshot={...liveSnapshot,sequence:3,capabilities:{...liveSnapshot.capabilities,liveTransport:true}};
coreReply(attachRequest,attachedSnapshot);
let playRequest=await nextRequest('command');assert.equal(playRequest.request.command.sequence,4);
const playingSnapshot={...attachedSnapshot,sequence:4};coreReply(playRequest,playingSnapshot);await livePlay;

const recovering=client.transport(playingSnapshot,{type:'recover'});
const recoveredContext=MockAudioContext.instances.at(-1);
assert.notEqual(recoveredContext,originalLiveContext);assert.equal(recoveredContext.resumeCalls,1);
const detachRequest=await nextRequest('detach_transport');await hostRequest('close');
assert.equal(originalLiveContext.closeCalls,1);assert.equal(recoveredContext.closeCalls,0);
coreReply(detachRequest,{...liveSnapshot,sequence:5});
attachRequest=await nextRequest('attach_transport');await hostRequest('open',{sampleRate:48000,channels:1});await hostRequest('reset');
coreReply(attachRequest,{...attachedSnapshot,sequence:6});
const stopRequest=await nextRequest('command');assert.equal(stopRequest.request.command.type,'stop');assert.equal(stopRequest.request.command.sequence,7);
coreReply(stopRequest,{...attachedSnapshot,sequence:7});await recovering;
client.dispose();client.dispose();await tick();
assert.equal(originalLiveContext.closeCalls,1);assert.equal(recoveredContext.closeCalls,1);
console.log(JSON.stringify({liveGestureBeforeAwait:true,cancelLiveSetupClosesOnce:true,recoveryFencesOldHostBeforeInstallingNewHost:true,liveDisposeClosesContextExactlyOnce:true}));
