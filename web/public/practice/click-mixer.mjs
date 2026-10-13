// Shared C++ click ABI adapter. Timing/onsets/accent decisions arrive from Go.
export class ClickMixer {
 constructor(module,channels,rate,level,windowEnd) {
  if (typeof module._riffine_click_mix !== 'function') throw new Error('Shared click processing is unavailable; rebuild browser DSP.');
  this.windowEnd=windowEnd;
  this.module=module;this.channels=channels;this.rate=rate;this.level=level;
  this.output=module._malloc(8192*channels*4);this.events=module._malloc(256*16);this.error=module._malloc(256);
  if (!this.output||!this.events||!this.error) {this.close();throw new Error('Click processing memory exhausted.');}
 }
 mix(pcm,start,clicks) {
  if (!this.output || !(pcm instanceof Float32Array) || pcm.length%this.channels || pcm.length/this.channels>8192 || !Number.isSafeInteger(start)||start<0 || !Array.isArray(clicks)||clicks.length>256) throw new Error('Invalid bounded click mix block.');
  const module=this.module,view=new DataView(module.HEAPU8.buffer);
  for (let i=0;i<clicks.length;i++) {
   const click=clicks[i];
   if (!Number.isSafeInteger(click.outputFrame)||click.outputFrame<0||typeof click.accent!=='boolean') throw new Error('Invalid core click event.');
   view.setBigInt64(this.events+i*16,BigInt(click.outputFrame),true);view.setUint32(this.events+i*16+8,click.accent?1:0,true);view.setUint32(this.events+i*16+12,0,true);
  }
  module.HEAPF32.set(pcm,this.output/4);
  const args=[1,this.channels,this.rate,BigInt(start),pcm.length/this.channels,this.level,this.events,clicks.length,this.output];
  let status;
  if(this.windowEnd!==undefined){
   if(typeof module._riffine_click_mix_window!=='function')throw new Error('Bounded click tails are unavailable; rebuild browser DSP.');
   status=module._riffine_click_mix_window(...args,BigInt(this.windowEnd),this.error,256);
  }else status=module._riffine_click_mix(...args,this.error,256);
  if (status) {
   const bytes=module.HEAPU8.subarray(this.error,this.error+256),end=bytes.indexOf(0);
   throw new Error(new TextDecoder().decode(bytes.subarray(0,end<0?256:end)));
  }
  pcm.set(module.HEAPF32.subarray(this.output/4,this.output/4+pcm.length));return pcm;
 }
 close(){for(const name of ['output','events','error']){if(this[name])this.module._free(this[name]);this[name]=0;}}
}
export class CueSelection {
 constructor(module,info,schedule) {
  this.infoSource=info;this.schedule=schedule;this.generated=0;this.frames=schedule.outputEnd-schedule.outputStart;
  if (schedule.phase==='count_in') this.mixer=new ClickMixer(module,info.channels,info.sampleRate,schedule.clickLevel,(schedule.cueOffsetFrames??0)+this.frames);
 }
 info(){return {outputFrames:this.frames};}
 read(capacity){
  const frames=Math.min(capacity,this.frames-this.generated),pcm=new Float32Array(frames*this.infoSource.channels);
  if(frames&&this.mixer)this.mixer.mix(pcm,(this.schedule.cueOffsetFrames??0)+this.generated,this.schedule.clicks??[]);
  this.generated+=frames;return {frames,pcm,needsInput:false};
 }
 close(){this.mixer?.close();}
}
