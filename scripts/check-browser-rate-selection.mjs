import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {Worker} from 'node:worker_threads';
const path = process.argv[2] ?? 'build/native/riffine-rate-selection-test';
const expected = execFileSync(path,{encoding:'utf8'}).trim().split('\n').map(line=>JSON.parse(line));
const worker = new Worker(new URL('./browser-selection-node-worker.mjs',import.meta.url));
let next=0,resolveReady,rejectReady;const pending=new Map();
const ready=new Promise((resolve,reject)=>{resolveReady=resolve;rejectReady=reject});
const timeout=setTimeout(()=>{rejectReady(new Error('Rate worker startup timed out'));worker.terminate()},30000);
worker.on('error',error=>{rejectReady(error);for(const p of pending.values())p.reject(error)});
worker.on('message',data=>{if(data.type==='ready'){clearTimeout(timeout);resolveReady(data)}else if(data.type==='fatal')rejectReady(new Error(data.message));else if(data.type==='result'){const p=pending.get(data.id);pending.delete(data.id);if(data.error)p?.reject(new Error(data.error));else p?.resolve(data.result)}});
const call=(type,fields={},transfers=[])=>new Promise((resolve,reject)=>{const id=++next;pending.set(id,{resolve,reject});worker.postMessage({id,type,...fields},transfers)});
async function run(reference) {
 const {sourceRate,outputRate,speed,pitch}=reference;
 const info=await call('create',{channels:2,sampleRate:sourceRate,outputRate,sourceFrames:sourceRate,speed,pitch});
 assert.equal(info.sampleRate,outputRate);assert.equal(info.outputFrames,reference.frames);
 if(sourceRate!==outputRate){assert.equal(info.sourceSampleRate,sourceRate);assert.equal(info.resamplerLatencyFrames,reference.latencyFrames)}
 let read=0,written=0,measured=0;const crossings=[0,0],energy=[0,0],previous=[0,0];
 for(;;){const {pcm,frames,needsInput}=await call('read',{capacity:113});assert(pcm instanceof Float32Array);assert.equal(pcm.length,frames*2);assert(frames<=113);
  for(let i=0;i<frames;i++){const measure=written+i>outputRate/10&&written+i+outputRate/10<info.outputFrames;
   for(let c=0;c<2;c++){const value=pcm[i*2+c];assert(Number.isFinite(value));if(measure){if(previous[c]<=0&&value>0)crossings[c]++;energy[c]+=value*value}previous[c]=value;if(written+i===0||written+i+1===info.outputFrames)assert(value===0)}if(measure)measured++
  }written+=frames;
  if(needsInput){const count=Math.min(512,sourceRate-read);assert(count>0);const pcm=new Float32Array(count*2);for(let i=0;i<count;i++)for(let c=0;c<2;c++)pcm[i*2+c]=.2*Math.sin(2*Math.PI*(c?660:440)*(read+i)/sourceRate);await call('push',{pcm},[pcm.buffer]);read+=count}
  else if(!frames)break;
 }
 assert.equal(read,sourceRate);assert.equal(written,reference.frames);assert.equal((await call('info')).consumedSourceFrames,read);
 for(let c=0;c<2;c++){const frequency=crossings[c]*outputRate/measured,rms=Math.sqrt(energy[c]/measured);assert(Math.abs(frequency-reference.frequency[c])<4);assert(Math.abs(rms-reference.rms[c])<.02)}
 const eof=await call('read',{capacity:1});assert.equal(eof.frames,0);assert.equal(eof.needsInput,false);await call('close');
}
try {
 const initial=await ready;assert.equal(initial.heapBytes,33554432);
 for(const outputRate of [0,7999,192001,Infinity,NaN])await assert.rejects(call('create',{channels:1,sampleRate:48000,outputRate,sourceFrames:48000,speed:1,pitch:0}));
 for(let iteration=0;iteration<2;iteration++)for(const reference of expected)await run(reference);
 for(const [sourceRate,outputRate] of [[8000,192000],[192000,8000],[48000,44100]]){
  const info=await call('create',{channels:1,sampleRate:sourceRate,outputRate,sourceFrames:1,speed:2,pitch:0});let fed=false,frames=0;
  for(;;){const block=await call('read',{capacity:1});frames+=block.frames;if(block.needsInput){assert(!fed);await assert.rejects(call('push',{pcm:new Float32Array([Infinity])}));assert.equal((await call('info')).consumedSourceFrames,0);await call('push',{pcm:new Float32Array([.2])});fed=true}else if(!block.frames)break}
  assert(fed);assert.equal(frames,info.outputFrames);await call('close');
 }
 const huge=await call('create',{channels:2,sampleRate:48000,outputRate:44100,sourceFrames:2**40,speed:.25,pitch:0});assert.equal(huge.outputFrames,Math.round(2**40*44100/48000/.25));await call('close');
 console.log(JSON.stringify({iterations:2,casesPerIteration:expected.length,fixedHeapBytes:initial.heapBytes,sourceBlockFrames:512,maxOutputBlockFrames:8192,nativeSignalParity:true,tinyRateCases:3,hugeRangeStorageBounded:true,rmsTolerance:.02},null,2));
}finally{clearTimeout(timeout);await worker.terminate()}
