// Original generated fixtures: real C++ decoder WASM, real Go binding; no personal music.
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {Worker} from 'node:worker_threads';
import {openCodec} from '../web/public/practice/codec-port.mjs';
const mediaOnly=process.argv.includes('--media-only');
const helper=process.argv.slice(2).find(arg=>arg!=='--media-only') ?? 'build/native/riffine-audio';
const open=(file,codec)=>new Promise((done,failed)=>openCodec(file,codec,done,error=>failed(new Error(error))));
const call=(port,method,args={})=>new Promise((done,failed)=>port.call(method,args,done,error=>failed(new Error(error))));
const results=[];
// Module workers expose importScripts even though calling it is forbidden.
// The packaged factory must leave our worker message handler in place.
const previousImportScripts=globalThis.importScripts, previousOnmessage=globalThis.onmessage;
const previousSelf=globalThis.self;
const receive=()=>{};
try {
 globalThis.importScripts=()=>{throw new Error('Module worker');};
 globalThis.onmessage=receive;
 globalThis.self={location:{href:'https://example.test/practice/codec-worker.mjs'}};
 await import('../web/public/practice/media/factory.mjs');
 await import('../web/public/practice/media/opus-factory.mjs');
 await Promise.resolve();
 assert.equal(globalThis.onmessage,receive,'media factory replaced the codec worker message handler');
} finally {globalThis.importScripts=previousImportScripts;globalThis.onmessage=previousOnmessage;globalThis.self=previousSelf;}
// Exercise the actual packaged 6.x allocator: a single 64 MiB allocation
// cannot fit alongside the decoder, while smaller allocations still work.
const {default:opusLibAV}=await import('../web/public/practice/media/opus-libav.mjs');
const {default:opusFactory}=await import('../web/public/practice/media/opus-factory.mjs');
const opusAV=await opusLibAV.LibAV({noworker:true,factory:opusFactory});
try {
 assert.equal(opusLibAV.CONFIG,'opus');
 assert.equal(await opusAV.malloc(64*1024*1024),0,'Opus allocator exceeded the memory budget');
 const allocation=await opusAV.malloc(1024*1024);assert(allocation,'Opus allocator failed below the budget');
 await opusAV.free(allocation);
} finally {opusAV.terminate();}
const manifest=JSON.parse(await readFile('native/testdata/generated-codecs.json','utf8'));
const formats=mediaOnly ? [['m4a',3],['alac.m4a',3],['opus',4]] : [['mp3',1],['flac',2],['m4a',3],['alac.m4a',3],['opus',4]];
for(const [suffix,codec] of formats) {
 const source=resolve(`native/testdata/generated-stereo.${suffix}`),bytes=new Uint8Array(await readFile(source));
 // The controlled Node File is cloned to the decoder worker. Browsers borrow a real File.
 const file={name:`音楽.${suffix}`,size:bytes.length,bytes};
 let expected,reference;
 const fixture=manifest.files.find(record=>record.name===`generated-stereo.${suffix}`);
 assert.equal(createHash('sha256').update(bytes).digest('hex'),fixture.sha256);
 if(codec<=2) {
  const nativeInfo=spawnSync(helper,['decoder-info',source],{encoding:'utf8'});assert.equal(nativeInfo.status,0,nativeInfo.stderr);
  expected=JSON.parse(nativeInfo.stdout);
  const nativePCM=spawnSync(helper,['decoder-read',source,'0']);assert.equal(nativePCM.status,0,nativePCM.stderr?.toString());
  reference=new Float32Array(nativePCM.stdout.buffer,nativePCM.stdout.byteOffset,nativePCM.stdout.byteLength/4);
 } else expected=fixture.audio;
 const {info,port}=await open(file,codec);
 let originalPCM;
 try {
  for(const key of ['encoding','channels','sampleRate','frames'])assert.equal(info[key],expected[key]);
  if(reference)assert.equal(reference.length,info.frames*info.channels);
  const decoded=new Float32Array(info.frames*info.channels);originalPCM=decoded;let offset=0;
  for(;;){const {frames,pcm}=await call(port,'read',{count:8192});if(!frames)break;const block=new Float32Array(pcm.buffer,pcm.byteOffset,pcm.byteLength/4);decoded.set(block,offset);offset+=block.length;}
  assert.equal(offset,decoded.length);let maxDifference=0;
  for(let i=0;i<decoded.length;i++){assert(Number.isFinite(decoded[i]));if(reference)maxDifference=Math.max(maxDifference,Math.abs(decoded[i]-reference[i]));}
  if(fixture.referenceSamples)for(const sample of fixture.referenceSamples)for(let c=0;c<info.channels;c++)maxDifference=Math.max(maxDifference,Math.abs(decoded[sample.frame*info.channels+c]-sample.channels[c]));
  assert(maxDifference<.00004,`${suffix} reference/WASM PCM difference ${maxDifference}`);
  reference??=decoded;
  for(const position of [18000,0,47999,12345,256,36000,48000]) {
   await call(port,'seek',{position});const {frames,pcm}=await call(port,'read',{count:512});assert.equal(frames,Math.min(512,info.frames-position));
   const block=new Float32Array(pcm.buffer,pcm.byteOffset,pcm.byteLength/4);
   assert.deepEqual(block,decoded.subarray(position*info.channels,(position+frames)*info.channels));
  }
  await assert.rejects(call(port,'seek',{position:info.frames+1}),/Invalid source seek/);
  results.push({codec:suffix,frames:info.frames,referencePCMMaximumDifference:maxDifference,exactRandomSeeks:true});
 } finally {await call(port,'close');}
 // A second cursor on the exact File reuses measured duration metadata. Its
 // full output, EOF and backwards seeks must still be sample-identical.
 const warm=await open(file,codec);
 try {
  assert.deepEqual(warm.info,info);
  let offset=0;
  for(;;){
   const {frames,pcm}=await call(warm.port,'read',{count:8192});if(!frames)break;
   const block=new Float32Array(pcm.buffer,pcm.byteOffset,pcm.byteLength/4);
   assert.deepEqual(block,originalPCM.subarray(offset,offset+block.length));offset+=block.length;
  }
  assert.equal(offset,info.frames*info.channels);
  await call(warm.port,'seek',{position:12345});
  const {frames,pcm}=await call(warm.port,'read',{count:512});
  assert.equal(frames,512);
  assert.deepEqual(new Float32Array(pcm.buffer,pcm.byteOffset,pcm.byteLength/4),originalPCM.subarray(12345*info.channels,12857*info.channels));
 } finally {await call(warm.port,'close');}
 results.at(-1).reusedInspectionExactPCM=true;
 // The actual Go resource/binding path retains the original SHA and saved source units.
 const worker=new Worker(new URL('./browser-practice-node-worker.mjs',import.meta.url),{workerData:{source,name:file.name,loader:resolve('web/public/practice/wasm_exec.js'),wasm:resolve('web/public/practice/core.wasm'),codecs:true,preview:!mediaOnly}});
 let next=0,readyResolve,readyReject;const pending=new Map();
 const ready=new Promise((done,failed)=>{readyResolve=done;readyReject=failed;});
 worker.on('error',error=>{readyReject(error);for(const item of pending.values())item.reject(error);});
 worker.on('message',message=>{if(message.type==='ready')readyResolve();if(message.type==='result'){const item=pending.get(message.id);pending.delete(message.id);item?.resolve(message);}});
 const request=(request,extra={})=>new Promise((resolve,reject)=>{const id=++next;pending.set(id,{resolve,reject});worker.postMessage({id,request:{version:1,...request},...extra});});
 let lastMessage;
 const response=async(requestFields,extra)=>{const message=await request(requestFields,extra);assert.equal(message.response.error,undefined);assert(message.maxRead<=65536);lastMessage=message;return message.response;};
 const deadline=setTimeout(()=>{readyReject(new Error('Codec binding timeout'));for(const item of pending.values())item.reject(new Error('Codec binding timeout'));worker.terminate();},30000);
 try {
  await ready;
  let result=await response({type:'import',source:'source',document:'project'},{file:'source'});
  const track=result.snapshot.project.track;
  const projectInfo={...expected};if(!projectInfo.bits)delete projectInfo.bits;
  assert.deepEqual(track.audio,{...projectInfo,sha256:createHash('sha256').update(bytes).digest('hex')});
  assert.deepEqual(result.snapshot.capabilities.formats,['wav-pcm','mp3','flac','aac','alac','opus']);
  const inspectedReads=lastMessage.reads;
  assert.equal(lastMessage.codecOpens,1);
  result=await response({type:'waveform',startFrame:0,endFrame:expected.frames,bins:32});assert(result.window.cells.length>0);
  assert.equal(lastMessage.codecOpens,2,'waveform should open only its PCM cursor');
  assert.equal(lastMessage.reads-inspectedReads,codec===2?1:0,'immutable File was fingerprinted again');
  result=await response({type:'detail',startFrame:12345,endFrame:12348,bins:3});assert.equal(result.window.cells.length,3);
  for(let i=0;i<3;i++)for(let c=0;c<expected.channels;c++)assert(Math.abs(result.window.cells[i].channels[c].min-reference[(12345+i)*expected.channels+c])<.00004);
  await response({type:'command',command:{version:1,sequence:1,type:'set_parameters',speed:.75,pitch:-2}});
  await response({type:'command',command:{version:1,sequence:2,type:'set_loop',loop:{enabled:true,start:.25,end:.75}}});
  const saved=(await response({type:'export'})).document;
  const mismatch=await request({type:'open',source:'source',document:'project'},{file:'changed',document:saved});
  assert.equal(mismatch.response.error?.code,'identity_mismatch','same-name, same-size replacement reused an old fingerprint');
  result=await response({type:'open',source:'source',document:'project'},{file:'source',document:saved});
  assert.equal(result.snapshot.project.settings.speed,.75);assert.equal(result.snapshot.project.settings.pitch,-2);assert.equal(result.snapshot.project.settings.loop.enabled,true);assert.equal(result.snapshot.project.track.audio.sha256,track.audio.sha256);
  // Cancel a new open while the separate decoder is initializing. Old session survives.
  const opening=request({type:'import',source:'source',document:'project'},{file:'source'});setTimeout(()=>worker.postMessage({type:'cancel'}),5);
  assert.equal((await opening).response.error?.code,'cancelled');
  result=await response({type:'snapshot'});assert.equal(result.snapshot.project.settings.speed,.75);
  results.at(-1).projectReopenAndOriginalSHA=true;results.at(-1).cancelledOpenPreservesSession=true;
 } finally {clearTimeout(deadline);await worker.terminate();}
}
const bad={name:'broken.mp3',size:4,bytes:new Uint8Array([73,68,51,0])};
if(!mediaOnly)await assert.rejects(open(bad,1),/Cannot decode/);
for(const [name,codec] of [['broken.m4a',3],['broken.opus',4]])await assert.rejects(open({...bad,name},codec));
// Immediate cancellation must release worker startup and reject the opening, then fence.
let aborted;
const cancelled=new Promise((done,failed)=>{aborted=openCodec(bad,mediaOnly?3:1,done,error=>failed(new Error(error)));});
const fence=new Promise(resolve=>aborted(resolve));await assert.rejects(cancelled,/cancelled/);assert.equal(await fence,'');
console.log(JSON.stringify({codecs:results,workerMessageHandlerPreserved:true,malformedRejected:true,cancellationFenced:true,wholeSongPCMBuffered:false},null,2));
