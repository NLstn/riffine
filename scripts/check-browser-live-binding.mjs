// Actual Go session/verified-reader factory and C++ worker processing. Device,
// FileReaderSync and AudioContext are platform shims; timestamps are synthetic.
import assert from 'node:assert/strict';
import { Worker } from 'node:worker_threads';
import { resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { LiveOutputHost } from '../web/public/practice/live-output-host.mjs';
const planMode=process.env.RIFFINE_LIVE_PLAN === '1';
const actionMode=process.env.RIFFINE_LIVE_ACTIONS === '1';
const speed=Number(process.env.RIFFINE_LIVE_SPEED ?? '.75');
const pitch=Number(process.env.RIFFINE_LIVE_PITCH ?? '3');
let Processor;
globalThis.AudioWorkletProcessor = class {};
globalThis.registerProcessor = (_name, value) => { Processor = value; };
await import('../web/public/practice/output-worklet.mjs');
class Context {
 constructor() { this.sampleRate=48000; this.state='running'; this.renderFrame=0; this.deviceFrame=0; this.closes=0; }
 getOutputTimestamp() { return {contextTime:this.deviceFrame/this.sampleRate, performanceTime:1}; }
 async suspend() { this.state='suspended'; }
 async resume() { this.state='running'; }
 async close() { this.closes++; this.state='closed'; }
}
class Node {
 constructor(_context,_name,options) { this.processor=new Processor(options); }
 connect() {} disconnect() {}
}
globalThis.AudioWorkletNode = Node;
let context, host, openingGate, openingEntered;
const worker = new Worker(new URL('./browser-practice-node-worker.mjs',import.meta.url), {workerData:{source:resolve('build/browser-conformance.wav'),name:'fixture.wav',loader:resolve('web/public/practice/wasm_exec.js'),wasm:resolve('web/public/practice/core.wasm'),live:true}});
let readyResolve,readyReject,next=0,sequence=0,maxRead=0;
const ready=new Promise((resolve,reject)=>{readyResolve=resolve;readyReject=reject;});
const pending=new Map();
const timeout=setTimeout(()=>{for(const call of pending.values()) call.reject(new Error('Live binding timed out.'));readyReject(new Error('Live binding timed out.'));worker.terminate();},planMode ? 60000 : 30000);
async function receiveHost(message) {
 try {
  let result={};
  switch(message.method) {
  case 'open': if(openingGate){openingEntered();await openingGate;}context=new Context();assert.equal(message.args.sampleRate,context.sampleRate);host=new LiveOutputHost(context,message.args.channels,()=>{});break;
  case 'reset': result=await host.reset();break;
  case 'start': await host.start();break;
  case 'suspend': if(host) await host.suspend();break;
  case 'fail': if(host) await host.fail(new Error(message.args.message));break;
  case 'close': if(host) await host.close();break;
  default: throw new Error('Unknown host operation: '+message.method);
  }
  worker.postMessage({type:'live_result',id:message.id,result});
 } catch(error) {worker.postMessage({type:'live_result',id:message.id,error:String(error)});}
}
worker.on('error',error=>{readyReject(error);for(const call of pending.values())call.reject(error);});
worker.on('message',message=>{
 maxRead=Math.max(maxRead,message.maxRead||0);
 if(message.type==='ready')readyResolve();
 if(message.type==='live_call')void receiveHost(message);
 if(message.type==='result'){const call=pending.get(message.id);pending.delete(message.id);call?.resolve(message.response);}
});
const call=request=>new Promise((resolve,reject)=>{const id=++next;pending.set(id,{resolve,reject});worker.postMessage({id,request:{version:1,...request},...(request.type==='import'?{file:'source'}:{})});});
const requireResponse=response=>{assert.equal(response.error,undefined);return response;};
const command=async fields=>{const response=requireResponse(await call({type:'command',command:{version:1,sequence:sequence+1,...fields}}));sequence=response.snapshot.sequence;return response.snapshot;};
const output=[[new Float32Array(128)]];
let pumping=false;
const pump=setInterval(()=>{
 if(!pumping||context?.state!=='running'||!host?.node)return;
 globalThis.currentFrame=context.renderFrame;
 host.node.processor.process([],output);context.renderFrame+=128;
 context.deviceFrame=Math.max(0,context.renderFrame-256);
},3);
async function loops(count) {
 const deadline=performance.now()+10000;
 for(;;){const snapshot=requireResponse(await call({type:'poll_transport'})).snapshot;if(snapshot.transport.completedLoops>=count)return snapshot;assert(performance.now()<deadline);await delay(20);}
}
try {
 await ready;
 let response=requireResponse(await call({type:'import',source:'source',document:'project'}));
 assert.equal(response.snapshot.capabilities.liveTransport,false);
 if (actionMode) {
  await command({type:'set_annotations',annotations:{notes:'Keep notes',regions:[{id:'one',label:'One',start:.25,end:.75},{id:'two',label:'Two',start:.1,end:.2}]}});
  response=requireResponse(await call({type:'attach_transport'}));sequence=response.snapshot.sequence;
  let snapshot=await command({type:'speed_up'});assert.equal(snapshot.project.settings.speed,1.05);
  snapshot=await command({type:'speed_down'});assert.equal(snapshot.project.settings.speed,1);
  await command({type:'play'});pumping=true;
  const deadline=performance.now()+10000;
  do {snapshot=requireResponse(await call({type:'poll_transport'})).snapshot;assert(performance.now()<deadline);await delay(10);} while(snapshot.transport.sourceFrame<12000);
  snapshot=await command({type:'pause'});const a=snapshot.transport.sourceFrame;
  snapshot=await command({type:'set_a'});assert.equal(snapshot.project.settings.loop.start,a/48000);assert.equal(snapshot.transport.config.loopStart,a);
  await command({type:'seek',position:36000});
  snapshot=await command({type:'set_b'});assert.equal(snapshot.project.settings.loop.end,.75);assert.equal(snapshot.transport.config.loopEnd,36000);
  await command({type:'seek',position:30000});
  snapshot=await command({type:'rewind_phrase'});assert.equal(snapshot.transport.sourceFrame,a);assert.equal(snapshot.state,'paused');
  snapshot=await command({type:'seek_forward'});assert.equal(snapshot.transport.sourceFrame,35999);
  snapshot=await command({type:'seek_backward'});assert.equal(snapshot.transport.sourceFrame,a);
  snapshot=await command({type:'next_region'});assert.equal(snapshot.transport.config.loopStart,12000);assert.equal(snapshot.state,'stopped');
  await command({type:'play'});
  snapshot=await command({type:'next_region'});assert.equal(snapshot.transport.config.loopStart,4800);assert.equal(snapshot.state,'playing');
  snapshot=await command({type:'next_region'});assert.equal(snapshot.transport.config.loopStart,12000);
  const definition={id:'ladder',label:'Ladder',regionId:'one',ladder:{startSpeed:.6,targetSpeed:1,incrementPoints:5,loopsPerStep:4,endBehavior:'stop'}};
  await command({type:'set_practice_plans',practicePlans:[definition]});
  await command({type:'select_practice_plan',planId:'ladder'});
  snapshot=await command({type:'speed_up'});assert.equal(snapshot.transport.config.speed,.65);assert.equal(snapshot.transport.plan.state,'paused');
  snapshot=await command({type:'practice_reset'});assert.equal(snapshot.state,'stopped');assert.equal(snapshot.activePracticePlan,undefined);assert.equal(snapshot.transport.plan,undefined);assert.equal(snapshot.transport.sourceFrame,0);
  const saved=JSON.parse(requireResponse(await call({type:'export'})).document);
  assert.equal(saved.settings.speed,1);assert.equal(saved.settings.pitch,0);assert.equal(saved.settings.loop.enabled,false);assert.equal(saved.annotations.notes,'Keep notes');assert.deepEqual(saved.practicePlans,[definition]);
  response=requireResponse(await call({type:'detach_transport'}));sequence=response.snapshot.sequence;assert.equal(context.closes,1);
  console.log(JSON.stringify({actualGoLiveFactory:true,actualCPPWASM:true,acknowledgedACapture:true,BCaptureSourceFrame:36000,rewindPreservesPause:true,boundedOneSecondSeeks:true,regionOrderAndPlayingState:true,fivePointSpeedSteps:true,stepPausesLadder:true,resetPreservesDefinitions:true,deviceClockIsSynthetic:true},null,2));
 } else if (planMode) {
  await command({type:'set_annotations',annotations:{regions:[{id:'phrase',label:'Phrase',start:.25,end:.75}]}});
  const definition={id:'ladder',label:'60–100',regionId:'phrase',ladder:{startSpeed:.6,targetSpeed:1,incrementPoints:5,loopsPerStep:4,endBehavior:'stop'}};
  await command({type:'set_practice_plans',practicePlans:[definition]});
  response=requireResponse(await call({type:'attach_transport'}));sequence=response.snapshot.sequence;
  let snapshot=await command({type:'select_practice_plan',planId:'ladder'});
  assert.equal(snapshot.transport.plan.totalSteps,9);assert.equal(snapshot.transport.plan.totalLoops,36);assert.equal(snapshot.state,'ready');assert.equal(context.state,'suspended');
  const selectedSequence=sequence;
  const invalid=await call({type:'command',command:{version:1,sequence:sequence+1,type:'select_practice_plan',planId:'missing'}});
  assert.equal(invalid.error.code,'invalid_command');assert.equal(invalid.snapshot.sequence,selectedSequence);
  await command({type:'play'});
  snapshot=requireResponse(await call({type:'poll_transport'})).snapshot;
  assert.equal(snapshot.transport.completedLoops,0);
  pumping=true;
  const completed=[];
  for(let ordinal=1;ordinal<=36;ordinal++) {
   snapshot=await loops(ordinal);
   const batch=requireResponse(await call({type:'events'})).events;
   assert.equal(batch.dropped,0);
   completed.push(...batch.events.filter(event=>event.type==='traversal_completed').map(event=>event.transport));
   if(ordinal===4) {
    await command({type:'set_practice_plans',practicePlans:[{...definition,label:'Updated definition',ladder:{...definition.ladder,targetSpeed:.8,loopsPerStep:1}}]});
    snapshot=requireResponse(await call({type:'poll_transport'})).snapshot;
    assert.equal(snapshot.activePracticePlan.label,'60–100');assert.equal(snapshot.transport.plan.totalLoops,36);
   }
  }
  assert.equal(snapshot.state,'stopped');assert.equal(snapshot.transport.plan.state,'completed');assert.equal(snapshot.transport.plan.completedLoops,36);
  assert.equal(completed.length,36);
  const expectedSpeeds=[.6,.65,.7,.75,.8,.85,.9,.95,1];
  completed.forEach((event,index)=>{assert.equal(event.completedLoops,index+1);assert.equal(event.speed,expectedSpeeds[Math.floor(index/4)]);assert.equal(event.startFrame,12000);assert.equal(event.endFrame,36000);});
  const exported=JSON.parse(requireResponse(await call({type:'export'})).document);
  assert.equal(exported.practicePlans[0].label,'Updated definition');assert.equal(exported.activePracticePlan,undefined);assert.equal(exported.transport,undefined);
  assert.equal(exported.settings.speed,1);assert.equal(exported.settings.loop.enabled,false);
  snapshot=await command({type:'restart'});assert.equal(snapshot.transport.plan.completedLoops,0);
  snapshot=await command({type:'set_parameters',speed:.8});assert.equal(snapshot.transport.plan.state,'paused');
  snapshot=await command({type:'resume_practice_plan'});assert.equal(snapshot.transport.plan.state,'running');assert.equal(snapshot.transport.config.speed,.6);assert.equal(snapshot.project.settings.speed,.8);
  snapshot=await command({type:'clear_practice_plan'});assert.equal(snapshot.state,'ready');assert.equal(snapshot.transport.plan,undefined);assert.equal(snapshot.activePracticePlan,undefined);assert.equal(snapshot.transport.config.loopEnabled,false);assert.equal(snapshot.transport.config.speed,.8);
  snapshot=await command({type:'select_practice_plan',planId:'ladder'});assert.equal(snapshot.transport.plan.totalLoops,5);assert.equal(snapshot.activePracticePlan.label,'Updated definition');
  response=requireResponse(await call({type:'detach_transport'}));sequence=response.snapshot.sequence;
  assert.equal(response.snapshot.activePracticePlan,undefined);assert.equal(context.closes,1);assert(maxRead<=65536);
  console.log(JSON.stringify({actualGoLiveFactory:true,actualCPPWASM:true,savedRegionSelected:true,plannedSteps:9,presentedLoops:completed.length,speeds:expectedSpeeds,queuedFramesDoNotAdvancePlan:true,definitionEditsKeepCurrentRun:true,manualOverrideAndExplicitResume:true,clearRestoresManualSettings:true,runtimeAbsentFromExport:true,maximumFileRead:maxRead,deviceClockIsSynthetic:true},null,2));
 } else {
 await command({type:'set_loop',loop:{enabled:true,start:.25,end:.75}});
 await command({type:'set_parameters',speed,pitch});
 response=requireResponse(await call({type:'attach_transport'}));sequence=response.snapshot.sequence;
 assert.equal(sequence,3);assert.equal(response.snapshot.capabilities.liveTransport,true);
 let snapshot=await command({type:'play'});
 assert.equal(snapshot.state,'playing');assert.equal(snapshot.transport.sourceFrame,12000);
 const notPresented=requireResponse(await call({type:'poll_transport'})).snapshot;
 assert.equal(notPresented.transport.completedLoops,0);assert.equal(notPresented.transport.consumedOutputFrame,0);
 pumping=true;
 snapshot=await loops(2);assert.equal(snapshot.transport.completedLoops,2);
 const previousGeneration=snapshot.transport.generation;
 snapshot=await command({type:'set_parameters',speed:.6,pitch:5});
 assert.equal(snapshot.state,'playing');assert.equal(snapshot.project.settings.speed,.6);assert.equal(snapshot.project.settings.pitch,5);
 assert(snapshot.transport.generation>previousGeneration);
 snapshot=await loops(snapshot.transport.completedLoops+1);
 snapshot=await command({type:'set_parameters',speed,pitch});
 assert.equal(snapshot.state,'playing');snapshot=await loops(snapshot.transport.completedLoops+1);
 snapshot=await command({type:'pause'});assert.equal(snapshot.state,'paused');
 const pausedFrame=snapshot.transport.sourceFrame,pausedLoops=snapshot.transport.completedLoops;
 assert(pausedFrame>=12000&&pausedFrame<=36000);assert(pausedLoops>=2);
 await delay(100);
 snapshot=requireResponse(await call({type:'poll_transport'})).snapshot;
 assert.equal(snapshot.transport.sourceFrame,pausedFrame);assert.equal(snapshot.transport.completedLoops,pausedLoops);
 await command({type:'seek',position:18000});await command({type:'play'});
 snapshot=await loops(1);assert.equal(snapshot.transport.completedLoops,1);
 snapshot=await command({type:'stop'});assert.equal(snapshot.state,'stopped');assert.equal(snapshot.transport.completedLoops,0);assert.equal(snapshot.transport.sourceFrame,12000);
 const events=requireResponse(await call({type:'events'})).events.events;
 assert(events.some(event=>event.type==='traversal_completed'));
 response=requireResponse(await call({type:'detach_transport'}));sequence=response.snapshot.sequence;
 assert.equal(response.snapshot.capabilities.liveTransport,false);assert.equal(context.closes,1);
 assert(maxRead<=65536);
 const saved=requireResponse(await call({type:'export'})).document;
 const project=JSON.parse(saved);assert.equal(project.settings.speed,speed);assert.equal(project.settings.pitch,pitch);assert.equal(project.settings.loop.start,.25);assert.equal(project.transport,undefined);
 // Fault the presentation clock after starting a real second generation. Error
 // reaches Go only after output closes, then a fresh attachment can play again.
 response=requireResponse(await call({type:'attach_transport'}));sequence=response.snapshot.sequence;
 await command({type:'play'});await loops(1);
 const faultedContext=context;
 context.getOutputTimestamp=()=>({contextTime:NaN,performanceTime:1});host.monitor.sample();await host.closing;
 response=await call({type:'poll_transport'});
 assert(response.error);assert.equal(response.snapshot.state,'error');assert.equal(faultedContext.state,'closed');assert.equal(faultedContext.closes,1);
 const faultSequence=response.snapshot.sequence;
 response=requireResponse(await call({type:'detach_transport'}));sequence=response.snapshot.sequence;
 assert.equal(sequence,faultSequence+1);
 response=requireResponse(await call({type:'attach_transport'}));sequence=response.snapshot.sequence;
 assert.notEqual(context,faultedContext);
 await command({type:'stop'});await command({type:'play'});snapshot=await loops(1);
 assert.equal(snapshot.transport.completedLoops,1);
 response=requireResponse(await call({type:'detach_transport'}));sequence=response.snapshot.sequence;assert.equal(context.closes,1);
 // Cancel after verification while the host-open RPC is held. The Go reply
 // must await host close before it can release the source callbacks/reader.
 let releaseOpening, enteredOpening;
 openingGate=new Promise(resolve=>{releaseOpening=resolve;});
 const entered=new Promise(resolve=>{enteredOpening=resolve;});openingEntered=enteredOpening;
 let cancelledReply=false;
 const cancelledAttach=call({type:'attach_transport'}).then(value=>{cancelledReply=true;return value;});
 await entered;worker.postMessage({type:'cancel'});await delay(40);
 assert.equal(cancelledReply,false);
 releaseOpening();response=await cancelledAttach;openingGate=undefined;
 assert.equal(response.error.code,'cancelled');assert.equal(response.snapshot.sequence,sequence);assert.equal(response.snapshot.capabilities.liveTransport,false);assert.equal(context.closes,1);
 response=requireResponse(await call({type:'attach_transport'}));sequence=response.snapshot.sequence;
 response=requireResponse(await call({type:'detach_transport'}));sequence=response.snapshot.sequence;assert.equal(context.closes,1);
 console.log(JSON.stringify({actualGoLiveFactory:true,actualCPPWASM:true,presentedLoops:pausedLoops,noQueuedFrameCompletion:true,pausedSourceFrame:pausedFrame,pausePreservesPosition:true,seekResumePartialTraversalNotCounted:true,stoppedResetsCounters:true,detachedClosesContext:true,successfulSequence:sequence,maximumFileRead:maxRead,deviceClockIsSynthetic:true,speed,pitch,clockFailureQuiescesBeforeGoError:true,freshAttachmentRecoversPlayback:true,cancelOpeningAwaitsSourceAndDeviceFence:true,liveParameterChangesResetGenerationAndContinue:true},null,2));
 }
} finally {clearInterval(pump);clearTimeout(timeout);await host?.close();await worker.terminate();}
