import { OutputConsumer } from './output-queue.mjs';
class RiffineOutput extends AudioWorkletProcessor {
 constructor(options) {
  super();
  this.consumer = new OutputConsumer(options.processorOptions.queue, options.processorOptions.timeline);
 }
 process(_inputs, outputs) {
  return this.consumer.process(outputs[0], globalThis.currentFrame);
 }
}
registerProcessor('riffine-output', RiffineOutput);
