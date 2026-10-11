import { parentPort, workerData } from 'node:worker_threads';
import { setImmediate as yieldWorker } from 'node:timers/promises';
import { OutputWriter } from '../web/public/practice/output-queue.mjs';
const writer = new OutputWriter(workerData.queue);
const block = new Float32Array(512 * 2);
let frame = 0, primed = false;
try {
 while (frame < workerData.frames) {
  const count = Math.min(512, workerData.frames - frame);
  for (let i = 0; i < count; i++) { const value = ((frame + i) % 1024 - 512) / 1024; block[i * 2] = value; block[i * 2 + 1] = -value; }
  if (writer.write(block.subarray(0, count * 2))) frame += count;
  else if (!primed) { primed = true; parentPort.postMessage('primed'); }
  await yieldWorker();
 }
 writer.sealEOF(); parentPort.postMessage('sealed');
} catch (error) { parentPort.postMessage({ error: String(error) }); }
