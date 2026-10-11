import assert from 'node:assert/strict';
import { createOutputQueue, OutputWriter, OutputConsumer, QueueState, CountersBusyError } from '../web/public/practice/output-queue.mjs';

function stereo(frames, begin = 0) {
 const pcm = new Float32Array(frames * 2);
 for (let i = 0; i < frames; i++) { pcm[i * 2] = (begin + i) / 10000; pcm[i * 2 + 1] = -(begin + i) / 10000; }
 return pcm;
}
const queue = createOutputQueue(2, 128);
const writer = new OutputWriter(queue), consumer = new OutputConsumer(queue);
const outputs = [new Float32Array(64), new Float32Array(64)];
assert(writer.write(stereo(128)));
assert.equal(writer.write(stereo(1)), false); // backpressure does not overwrite
assert.equal(consumer.process(outputs), true);
assert(outputs.every(channel => channel.every(value => value === 0)));
assert.equal(writer.snapshot().copiedFrames, 0);
writer.start();
assert.equal(consumer.process(outputs), true);
for (let c = 0; c < 2; c++) for (let i = 0; i < 64; i++) assert.equal(outputs[c][i], stereo(128)[i * 2 + c]);
assert(writer.write(stereo(64, 128)));
assert.equal(consumer.process(outputs), true);
for (let c = 0; c < 2; c++) for (let i = 0; i < 64; i++) assert.equal(outputs[c][i], stereo(128)[(i + 64) * 2 + c]);
writer.sealEOF();
assert.equal(consumer.process(outputs), false);
assert.equal(writer.snapshot().copiedFrames, 192);
assert.equal(writer.snapshot().underrunFrames, 0);
assert.equal(writer.snapshot().state, QueueState.done);
assert.throws(() => writer.write(stereo(1)));
assert.throws(() => writer.start());

const starving = createOutputQueue(1, 128), starvingWriter = new OutputWriter(starving), starvingConsumer = new OutputConsumer(starving);
const mono = [new Float32Array(128)];
starvingWriter.start(); starvingConsumer.process(mono);
assert.equal(starvingWriter.snapshot().underrunFrames, 128);
assert.equal(starvingWriter.snapshot().copiedFrames, 0);
assert.throws(() => starvingWriter.write(new Float32Array([Infinity])));
assert.equal(starvingWriter.snapshot().copiedFrames, 0);
starvingWriter.close(); assert.equal(starvingConsumer.process(mono), false);
assert(mono[0].every(value => value === 0));

// Queue cursors and low/high submission counters survive uint32 wrap. Seed only
// in this controlled test; real hosts never write consumer-owned fields.
const wrap = createOutputQueue(1, 128), wrapWriter = new OutputWriter(wrap), wrapConsumer = new OutputConsumer(wrap);
const header = new Int32Array(wrap, 0, 16);
header[3] = -16; header[4] = -16; header[7] = -16; header[8] = 2;
assert(wrapWriter.write(new Float32Array(32).fill(.25)));
wrapWriter.sealEOF(); wrapWriter.start(); assert.equal(wrapConsumer.process(mono), false);
assert.equal(wrapWriter.snapshot().copiedFrames, 3 * 2 ** 32 + 16);
for (let i = 0; i < 128; i++) assert.equal(mono[0][i], i < 32 ? .25 : 0);

const malformed = createOutputQueue(1, 128), malformedWriter = new OutputWriter(malformed), malformedConsumer = new OutputConsumer(malformed);
malformedWriter.start(); new Int32Array(malformed, 0, 16)[3] = 129;
assert.equal(malformedConsumer.process(mono), false);
assert.equal(malformedWriter.snapshot().state, QueueState.error);
assert(mono[0].every(value => value === 0));
const busy = createOutputQueue(1, 128), busyWriter = new OutputWriter(busy);
new Int32Array(busy, 0, 16)[9] = 1;
assert.throws(() => busyWriter.snapshot(), CountersBusyError);
assert.throws(() => createOutputQueue(3));
assert.throws(() => createOutputQueue(2, 129));
console.log(JSON.stringify({ boundedFrames: 128, backpressure: true, zeroFilledUnderruns: true, uint32Wrap: true, terminalTail: true, corruptCursorStopsOutput: true, submissionOnlyCounters: true }, null, 2));

// Run the actual production AudioWorkletProcessor through a platform transport
// shim, with a separate writer thread exercising publication/backpressure races.
const { Worker } = await import('node:worker_threads');
const { setImmediate: yieldConsumer } = await import('node:timers/promises');
let Processor;
globalThis.AudioWorkletProcessor = class {};
globalThis.registerProcessor = (name, processor) => { assert.equal(name, 'riffine-output'); Processor = processor; };
await import('../web/public/practice/output-worklet.mjs');
const concurrent = createOutputQueue(2, 32768), concurrentWriter = new OutputWriter(concurrent);
const processor = new Processor({ processorOptions: { queue: concurrent } });
const frames = 250013;
const producer = new Worker(new URL('./browser-output-producer-worker.mjs', import.meta.url), { workerData: { queue: concurrent, frames } });
let rejectProducer;
const primed = new Promise((resolve, reject) => {
 rejectProducer = reject;
 producer.on('error', reject);
 producer.on('message', message => { if (message?.error) reject(new Error(message.error)); if (message === 'primed') resolve(); });
});
const deadline = setTimeout(() => { rejectProducer(new Error('Concurrent output test timed out.')); producer.terminate(); }, 10000);
try {
 await primed;
 concurrentWriter.start();
 const output = [new Float32Array(128), new Float32Array(128)], graphOutput = [output];
 const input = [];
 let consumed = 0;
 for (;;) {
  const before = concurrentWriter.snapshot().copiedFrames;
  const active = processor.process(input, graphOutput);
  const after = concurrentWriter.snapshot().copiedFrames;
  const copied = after - before;
  for (let i = 0; i < copied; i++) {
   const value = ((consumed + i) % 1024 - 512) / 1024;
   assert(output[0][i] === value && output[1][i] === -value);
  }
  for (let i = copied; i < 128; i++) assert(output[0][i] === 0 && output[1][i] === 0);
  consumed += copied;
  if (!active) break;
  await yieldConsumer();
 }
 assert.equal(consumed, frames);
 assert.equal(concurrentWriter.snapshot().state, QueueState.done);
 console.log(JSON.stringify({ concurrentFrames: frames, productionWorkletShim: true, exactPCMOrder: true, underrunFrames: concurrentWriter.snapshot().underrunFrames }, null, 2));
} finally { clearTimeout(deadline); await producer.terminate(); }
