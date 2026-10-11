import { OutputConsumer } from './output-queue.mjs';
class RiffineOutput extends AudioWorkletProcessor {
 constructor(options) {
  super();
  this.consumer = new OutputConsumer(options.processorOptions.queue);
 }
 process(_inputs, outputs) {
  return this.consumer.process(outputs[0]);
 }
}
registerProcessor('riffine-output', RiffineOutput);
