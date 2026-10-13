// Compare the same shared click ABI built natively and for WASM.
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {resolve} from 'node:path';
import factory from '../web/public/practice/dsp.mjs';
const executable=resolve(process.argv[2]??'build/native/riffine-click-test');
const native=spawnSync(executable,[],{env:{...process.env,RIFFINE_CLICK_FIXTURE:'1'},encoding:'utf8'});
assert.equal(native.status,0,native.stderr);
const expected=JSON.parse(native.stdout);
const module=await factory(),frames=2048,channels=2;
assert.equal(typeof module._riffine_click_mix,'function');
const output=module._malloc(frames*channels*4),events=module._malloc(32),error=module._malloc(256);
try {
 const data=new DataView(module.HEAPU8.buffer);
 data.setBigInt64(events,100n,true);data.setUint32(events+8,1,true);data.setUint32(events+12,0,true);
 data.setBigInt64(events+16,960n,true);data.setUint32(events+24,0,true);data.setUint32(events+28,0,true);
 module.HEAPF32.fill(0,output/4,output/4+frames*channels);
 assert.equal(module._riffine_click_mix(1,channels,48000,0n,frames,1,events,2,output,error,256),0);
 const actual=Array.from(module.HEAPF32.subarray(output/4,output/4+frames*channels));
 let maxError=0;
 for(let i=0;i<actual.length;i++)maxError=Math.max(maxError,Math.abs(actual[i]-expected[i]));
 assert(maxError<=1e-6,`Native/WASM click error ${maxError}`);
 module.HEAPF32.fill(0,output/4,output/4+frames*channels);
 for(let start=0;start<frames;start+=127)assert.equal(module._riffine_click_mix(1,channels,48000,BigInt(start),Math.min(127,frames-start),1,events,2,output+start*channels*4,error,256),0);
 assert.deepEqual(Array.from(module.HEAPF32.subarray(output/4,output/4+frames*channels)),actual);
 const boundary=spawnSync(executable,[],{env:{...process.env,RIFFINE_CLICK_BOUNDARY_FIXTURE:'1'},encoding:'utf8'});assert.equal(boundary.status,0,boundary.stderr);
 const boundaryExpected=JSON.parse(boundary.stdout);
 data.setBigInt64(events,180n,true);data.setUint32(events+8,0,true);
 module.HEAPF32.fill(0,output/4,output/4+200);
 assert.equal(module._riffine_click_mix_window(1,1,48000,0n,200,1,events,1,output,200n,error,256),0);
 const boundaryActual=Array.from(module.HEAPF32.subarray(output/4,output/4+200));
 boundaryActual.forEach((sample,i)=>assert(Math.abs(sample-boundaryExpected[i])<=1e-6));assert(Math.abs(boundaryActual.at(-1))<.002);
 // Restore original fixture metadata for the no-growth probe.
 data.setBigInt64(events,100n,true);data.setUint32(events+8,1,true);
 const before=module.HEAPU8.buffer.byteLength;
 for(let i=0;i<100;i++)assert.equal(module._riffine_click_mix(1,channels,48000,0n,frames,0,events,2,output,error,256),0);
 assert.equal(module.HEAPU8.buffer.byteLength,before);
 console.log(JSON.stringify({nativeWASMClickParity:true,maxAbsoluteSampleError:maxError,chunkIndependentPhase:true,boundedTailParity:true,fixedHeapBytes:before}));
} finally {module._free(output);module._free(events);module._free(error);}
