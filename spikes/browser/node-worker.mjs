import { parentPort, workerData } from 'node:worker_threads';
import { readFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
import { performance } from 'node:perf_hooks';
const started=performance.now();
await import(pathToFileURL(`${workerData.output}/wasm_exec.js`));
const go=new globalThis.Go();
const wasm=await readFile(`${workerData.output}/core.wasm`);
const {instance}=await WebAssembly.instantiate(wasm,go.importObject);
go.run(instance).catch(error => {throw error;});
if(typeof globalThis.riffineRunFixture!=='function') throw new Error('Go bridge did not initialize');
parentPort.postMessage({type:'ready',startupMs:performance.now()-started,memoryBytes:instance.exports.mem.buffer.byteLength});
parentPort.on('message',data => {
  const start=performance.now();
  const report=JSON.parse(globalThis.riffineRunFixture(data.fixture));
  parentPort.postMessage({type:'result',id:data.id,report,runMs:performance.now()-start,memoryBytes:instance.exports.mem.buffer.byteLength});
});
