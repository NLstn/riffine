// Actual shared DSP WASM: live changes keep source/DSP history, preserve exact
// output boundaries, and retain a pitched signal without an inserted silent gap.
import assert from 'node:assert/strict';
import createModule from '../web/public/practice/dsp.mjs';
import {Selection} from '../web/public/practice/selection.mjs';
const module=await createModule();
let cases=0,updates=0;
for(const rate of [8000,44100,48000,96000])for(const initial of [.25,.75,1,2])for(const target of [.25,.6,1,2]){
 const frames=rate*3,selection=new Selection(module,2,rate,frames,initial,0,0);
 let read=0,written=0,change,energy=0,crossings=0,measured=0,previous=0;
 try{
  const initialInfo=selection.info();
  assert.equal(selection.configure(target,3),undefined,'unprimed update must decline');
  assert.deepEqual(selection.info(),initialInfo);
  assert.throws(()=>selection.configure(0,3));
  assert.deepEqual(selection.info(),initialInfo,'invalid parameters mutated source/output counts');
  for(;;){
   const result=selection.read(4096);
   for(let i=0;i<result.frames;i++){
    const value=result.pcm[i*2];assert(Number.isFinite(value));
    if(change&&written+i>change.outputFrame+rate/4&&written+i<change.outputFrames-rate/4){
     energy+=value*value;measured++;if(previous<=0&&value>0)crossings++;
    }
    previous=value;
   }
   written+=result.frames;
   if(result.needsInput){
    if(!change&&written>rate/8){
     change=selection.configure(target,3);
     if(change){updates++;assert.equal(change.outputFrames,change.outputFrame+Math.round((frames-change.sourceFrame)/target));assert.equal(selection.info().consumedSourceFrames,read);}
    }
    const count=Math.min(512,frames-read),pcm=new Float32Array(count*2);
    for(let i=0;i<count;i++)pcm[i*2]=pcm[i*2+1]=.2*Math.sin(2*Math.PI*440*(read+i)/rate);
    selection.push(pcm);read+=count;
   }else if(!result.frames)break;
  }
  assert(change);assert.equal(read,frames);assert.equal(written,change.outputFrames);
  assert.equal(selection.configure(initial,0),undefined,'flushed cursor must decline');
  assert(measured>0);assert(Math.abs(crossings*rate/measured-440*2**(3/12))<6);
  assert(Math.sqrt(energy/measured)>.05,'live change lost its audible signal');
  cases++;
 }finally{selection.close();}
}
// Many changes on one cursor, including fractional speeds and both extremes.
const selection=new Selection(module,1,48000,48000*10,1,0,0);
let read=0,written=0,last=0,count=0;
try{
 for(;;){const result=selection.read(113);written+=result.frames;
  if(result.needsInput){
   if(written-last>3000&&count<40){const change=selection.configure([.25,.37,.83,1.3,2][count%5],count%2?3:-2);if(change){last=written;count++;}}
   const frames=Math.min(512,480000-read),pcm=new Float32Array(frames);for(let i=0;i<frames;i++)pcm[i]=.2*Math.sin(2*Math.PI*440*(read+i)/48000);
   selection.push(pcm);read+=frames;
  }else if(!result.frames)break;
 }
 assert.equal(read,480000);assert.equal(written,selection.info().outputFrames);assert.equal(count,40);
}finally{selection.close();}
console.log(JSON.stringify({actualDSPWASM:true,rateAndSpeedCases:cases,parameterUpdates:updates+count,exactDurations:true,pitchedSignalRetained:true,sourceHistoryPreserved:true}));
