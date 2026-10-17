import assert from 'node:assert/strict';
import {Worker} from 'node:worker_threads';
import {resolve} from 'node:path';
import {createRequire} from 'node:module';
import {AnalysisCache, PeakCache} from '../web/public/practice/project-store.mjs';
const require=createRequire(new URL('../web/package.json',import.meta.url));
const indexedDB=new (require('fake-indexeddb').IDBFactory)();
const cache=new AnalysisCache({indexedDB}), reopened=new AnalysisCache({indexedDB});
const data=new TextEncoder().encode('{"frames":48000}');
await cache.put('inspection-v1-hash',data);
assert.deepEqual(await reopened.get('inspection-v1-hash'),data);
assert.equal(await reopened.get('inspection-v2-hash'),null);
assert.equal(await reopened.get('inspection-v1-changed'),null);
await assert.rejects(cache.put('oversized',new Uint8Array(4097)));
assert.equal(await new PeakCache({indexedDB}).get('inspection-v1-hash'),null,'analysis must be isolated from peaks');
await assert.rejects(new AnalysisCache({indexedDB:null}).get('key'));
const worker=new Worker(new URL('./browser-practice-node-worker.mjs',import.meta.url),{workerData:{
 source:resolve('native/testdata/generated-stereo.m4a'),name:'same-name.m4a',
 loader:resolve('web/public/practice/wasm_exec.js'),wasm:resolve('web/public/practice/core.wasm'),
 codecs:true,analysisCache:true,
}});
let next=0;
const pending=new Map();
let readyResolve,readyReject;
const ready=new Promise((resolve,reject)=>{readyResolve=resolve;readyReject=reject;});
worker.on('message',message=>{
 if(message.type==='ready')readyResolve();
 if(message.type==='result'||message.type==='analysis_result'){pending.get(message.id)?.resolve(message);pending.delete(message.id);}
});
worker.on('error',error=>{readyReject(error);for(const call of pending.values())call.reject(error);});
const request=(request,file)=>new Promise((resolve,reject)=>{const id=++next;pending.set(id,{resolve,reject});worker.postMessage({id,request:{version:1,...request},file});});
const hostAnalysis=(method,args)=>new Promise((resolve,reject)=>{const id=++next;pending.set(id,{resolve,reject});worker.postMessage({id,type:'host_analysis',method,args});});
const importSource=()=>request({type:'import',source:'source',document:'project'},'source');
const deadline=setTimeout(()=>{const error=new Error('Analysis cache test timed out');readyReject(error);for(const call of pending.values())call.reject(error);},30000);
try {
 await ready;
 const cold=await importSource();
 assert.equal(cold.response.error,undefined);assert.equal(cold.codecOpens,1);
 const frames=cold.response.snapshot.project.track.audio.frames;
 const waveform=await request({type:'waveform',startFrame:0,endFrame:frames,bins:32});
 assert.equal(waveform.response.error,undefined);assert.equal(waveform.codecOpens,2);
 assert.equal(waveform.knownInfoOpens,1,'PCM cursor should reuse verified inspection');
 // New File and Files owner, same original bytes: persistent duration + peaks.
 const warm=await importSource();
 assert.equal(warm.response.error,undefined);
 assert.equal(warm.codecOpens,2,'warm import repeated decoder duration analysis');
 assert.deepEqual(warm.response.snapshot.project.track.audio,cold.response.snapshot.project.track.audio);
 const warmWave=await request({type:'waveform',startFrame:0,endFrame:frames,bins:32});
 assert.equal(warmWave.response.error,undefined);
 assert.equal(warmWave.codecOpens,2,'warm waveform decoded the song again');
 assert.deepEqual(warmWave.response.window,waveform.response.window);
 const detail=await request({type:'detail',startFrame:12345,endFrame:12348,bins:3});
 assert.equal(detail.response.error,undefined);assert.equal(detail.knownInfoOpens,2);
 const key='inspection-v1-'+cold.response.snapshot.project.track.audio.sha256;
 const stored=await hostAnalysis('get',[key]);
 const corrupt=JSON.parse(new TextDecoder().decode(stored.result));
 corrupt.info.frames++;
 await hostAnalysis('put',[key,new TextEncoder().encode(JSON.stringify(corrupt))]);
 const recovered=await importSource();
 assert.equal(recovered.response.error,undefined);
 assert.equal(recovered.codecOpens,4,'corrupt metadata was not rebuilt');
 assert.deepEqual(recovered.response.snapshot.project.track.audio,cold.response.snapshot.project.track.audio);
 const changed=await request({type:'import',source:'source',document:'project'},'changed');
 assert.equal(changed.response.error,undefined);
 assert.notEqual(changed.response.snapshot.project.track.audio.sha256,cold.response.snapshot.project.track.audio.sha256);
 assert.equal(changed.codecOpens,5,'same-name replacement incorrectly reused analysis');
 console.log('Persistent analysis: fresh owners, version/content misses, limits, unavailable storage, warm import, cached waveform, corrupt-entry recovery and verified detail passed');
} finally {clearTimeout(deadline);await worker.terminate();}
