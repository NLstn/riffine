// Repeatable native/WASM bridge comparison. Node is not a browser/device claim.
import { Worker } from 'node:worker_threads';
import { readFile, stat } from 'node:fs/promises';
import { resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import { deepStrictEqual, equal } from 'node:assert';
import { performance } from 'node:perf_hooks';
const output=resolve(process.argv[2] ?? 'build/host-spike');
const fixture=await readFile(`${output}/fixture.json`,'utf8');
const native=spawnSync(`${output}/host-spike`,{input:fixture,encoding:'utf8'});
if(native.status!==0)throw new Error(native.stderr);
const expected=JSON.parse(native.stdout);equal(expected.ok,true,expected.error);
const worker=new Worker(new URL('./node-worker.mjs',import.meta.url),{workerData:{output}});
const metrics={runtime:process.version,wasmBytes:(await stat(`${output}/core.wasm`)).size,iterations:20,runsMs:[],roundTripsMs:[]};
let index=0,sent;
const timeout=setTimeout(()=>{console.error('Worker timed out');process.exitCode=1;worker.terminate();},30000);
worker.on('error',error=>{clearTimeout(timeout);console.error(error);process.exitCode=1;worker.terminate();});
worker.on('message',data=>{
 try{
  if(data.type==='ready'){metrics.startupMs=data.startupMs;metrics.initialMemoryBytes=data.memoryBytes;sent=performance.now();worker.postMessage({id:++index,fixture});}
  if(data.type==='result'){
   deepStrictEqual(data.report,expected);equal(data.id,index);
   metrics.runsMs.push(data.runMs);metrics.roundTripsMs.push(performance.now()-sent);metrics.finalMemoryBytes=data.memoryBytes;
   if(index<metrics.iterations){sent=performance.now();worker.postMessage({id:++index,fixture});}
   else{clearTimeout(timeout);console.log(JSON.stringify(metrics,null,2));worker.terminate();}
  }
 }catch(error){clearTimeout(timeout);console.error(error);process.exitCode=1;worker.terminate();}
});
