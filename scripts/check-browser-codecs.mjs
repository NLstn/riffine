// Original generated fixtures: real C++ decoder WASM, real Go binding; no personal music.
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {createHash} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {Worker} from 'node:worker_threads';
import {openCodec} from '../web/public/practice/codec-port.mjs';
const helper=process.argv[2] ?? 'build/native/riffine-audio';
const open=(file,codec)=>new Promise((done,failed)=>openCodec(file,codec,done,error=>failed(new Error(error))));
const call=(port,method,args={})=>new Promise((done,failed)=>port.call(method,args,done,error=>failed(new Error(error))));
const results=[];
for(const [suffix,codec] of [['mp3',1],['flac',2]]) {
 const source=resolve(`native/testdata/generated-stereo.${suffix}`),bytes=new Uint8Array(await readFile(source));
 // The controlled Node File is cloned to the decoder worker. Browsers borrow a real File.
 const file={name:`音楽.${suffix}`,size:bytes.length,bytes};
 const nativeInfo=spawnSync(helper,['decoder-info',source],{encoding:'utf8'});assert.equal(nativeInfo.status,0,nativeInfo.stderr);
 const expected=JSON.parse(nativeInfo.stdout);
 const nativePCM=spawnSync(helper,['decoder-read',source,'0']);assert.equal(nativePCM.status,0,nativePCM.stderr?.toString());
 const reference=new Float32Array(nativePCM.stdout.buffer,nativePCM.stdout.byteOffset,nativePCM.stdout.byteLength/4);
 const {info,port}=await open(file,codec);
 try {
  for(const key of ['encoding','channels','sampleRate','frames'])assert.equal(info[key],expected[key]);
  assert.equal(reference.length,info.frames*info.channels);
  const decoded=new Float32Array(reference.length);let offset=0;
  for(;;){const {frames,pcm}=await call(port,'read',{count:8192});if(!frames)break;const block=new Float32Array(pcm.buffer,pcm.byteOffset,pcm.byteLength/4);decoded.set(block,offset);offset+=block.length;}
  assert.equal(offset,decoded.length);let maxDifference=0;
  for(let i=0;i<decoded.length;i++){assert(Number.isFinite(decoded[i]));maxDifference=Math.max(maxDifference,Math.abs(decoded[i]-reference[i]));}
  assert(maxDifference<.00004,`${suffix} native/WASM PCM difference ${maxDifference}`);
  for(const position of [18000,0,47999,12345,256,36000,48000]) {
   await call(port,'seek',{position});const {frames,pcm}=await call(port,'read',{count:512});assert.equal(frames,Math.min(512,info.frames-position));
   const block=new Float32Array(pcm.buffer,pcm.byteOffset,pcm.byteLength/4);
   assert.deepEqual(block,decoded.subarray(position*info.channels,(position+frames)*info.channels));
  }
  await assert.rejects(call(port,'seek',{position:info.frames+1}),/Invalid source seek/);
  results.push({codec:suffix,frames:info.frames,nativePCMMaximumDifference:maxDifference,exactRandomSeeks:true});
 } finally {await call(port,'close');}
 // The actual Go resource/binding path retains the original SHA and saved source units.
 const worker=new Worker(new URL('./browser-practice-node-worker.mjs',import.meta.url),{workerData:{source,name:file.name,loader:resolve('web/public/practice/wasm_exec.js'),wasm:resolve('web/public/practice/core.wasm'),preview:true}});
 let next=0,readyResolve,readyReject;const pending=new Map();
 const ready=new Promise((done,failed)=>{readyResolve=done;readyReject=failed;});
 worker.on('error',error=>{readyReject(error);for(const item of pending.values())item.reject(error);});
 worker.on('message',message=>{if(message.type==='ready')readyResolve();if(message.type==='result'){const item=pending.get(message.id);pending.delete(message.id);item?.resolve(message);}});
 const request=(request,extra={})=>new Promise((resolve,reject)=>{const id=++next;pending.set(id,{resolve,reject});worker.postMessage({id,request:{version:1,...request},...extra});});
 const response=async(requestFields,extra)=>{const message=await request(requestFields,extra);assert.equal(message.response.error,undefined);assert(message.maxRead<=65536);return message.response;};
 const deadline=setTimeout(()=>{readyReject(new Error('Codec binding timeout'));for(const item of pending.values())item.reject(new Error('Codec binding timeout'));worker.terminate();},30000);
 try {
  await ready;
  let result=await response({type:'import',source:'source',document:'project'},{file:'source'});
  const track=result.snapshot.project.track;
  const projectInfo={...expected};if(!projectInfo.bits)delete projectInfo.bits;
  assert.deepEqual(track.audio,{...projectInfo,sha256:createHash('sha256').update(bytes).digest('hex')});
  assert.deepEqual(result.snapshot.capabilities.formats,['wav-pcm','mp3','flac']);
  result=await response({type:'waveform',startFrame:0,endFrame:expected.frames,bins:32});assert(result.window.cells.length>0);
  result=await response({type:'detail',startFrame:12345,endFrame:12348,bins:3});assert.equal(result.window.cells.length,3);
  for(let i=0;i<3;i++)for(let c=0;c<expected.channels;c++)assert(Math.abs(result.window.cells[i].channels[c].min-reference[(12345+i)*expected.channels+c])<.00004);
  await response({type:'command',command:{version:1,sequence:1,type:'set_parameters',speed:.75,pitch:-2}});
  await response({type:'command',command:{version:1,sequence:2,type:'set_loop',loop:{enabled:true,start:.25,end:.75}}});
  const saved=(await response({type:'export'})).document;
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
await assert.rejects(open(bad,1),/Cannot decode/);
// Immediate cancellation must release worker startup and reject the opening, then fence.
let aborted;
const cancelled=new Promise((done,failed)=>{aborted=openCodec(bad,1,done,error=>failed(new Error(error)));});
const fence=new Promise(resolve=>aborted(resolve));await assert.rejects(cancelled,/cancelled/);assert.equal(await fence,'');
console.log(JSON.stringify({codecs:results,malformedRejected:true,cancellationFenced:true,wholeSongPCMBuffered:false},null,2));
