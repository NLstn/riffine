import { parentPort, workerData } from 'node:worker_threads';
import { createOutputQueue, OutputWriter } from '../web/public/practice/output-queue.mjs';
let Processor;
globalThis.AudioWorkletProcessor = class {};
globalThis.registerProcessor = (_name, processor) => { Processor = processor; };
await import('../web/public/practice/output-worklet.mjs');
const queue = createOutputQueue(1, 128), writer = new OutputWriter(queue);
const processor = new Processor({ processorOptions: { queue, timeline: workerData.timeline } });
const output = [[new Float32Array(128)]], input = [];
const full = new Float32Array(128).fill(.25), half = new Float32Array(64).fill(.5);
let contextFrame = 0, copiedFrames = 0;
writer.start();
try {
 for (let batch = 0; batch < 128; batch++) {
  for (let i = 0; i < 64; i++) {
   globalThis.currentFrame = contextFrame;
   const block = i % 2 ? half : full;
   writer.write(block);
   if (!processor.process(input, output)) throw new Error('Unexpected worklet stop.');
   if (i % 11 === 0) {
    const before = writer.snapshot();
    const original = output[0][0].slice();
    if (!processor.process(input, output)) throw new Error('Repeated worklet pull stopped.');
    const after = writer.snapshot();
    if (before.copiedFrames !== after.copiedFrames || before.underrunFrames !== after.underrunFrames || original.some((sample,index) => sample !== output[0][0][index])) throw new Error('Repeated worklet pull changed PCM or counters.');
   }
   copiedFrames += block.length; contextFrame += 128;
   if (i % 7 === 0) {
    globalThis.currentFrame = contextFrame;
    if (!processor.process(input, output)) throw new Error('Unexpected underrun stop.');
    contextFrame += 128;
   }
  }
  const retired = new Promise(resolve => parentPort.once('message', resolve));
  parentPort.postMessage({ contextEnd: contextFrame, copiedFrames, blocks: 64 });
  await retired; // test harness barrier outside the audio callback
 }
 parentPort.postMessage({ done: true });
} catch (error) { parentPort.postMessage({ error: error.message }); }
