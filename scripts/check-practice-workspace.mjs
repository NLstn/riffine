// Render the actual workspace under React Strict Mode, including effect replay.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdtemp, rm} from 'node:fs/promises';
import {fileURLToPath, pathToFileURL} from 'node:url';

const require=createRequire(new URL('../web/package.json',import.meta.url));
const {JSDOM}=require('jsdom');
const dom=new JSDOM('<div id="root"></div>',{url:'http://localhost/'});
for(const name of ['window','document','Element','HTMLElement','localStorage'])globalThis[name]=name==='window'?dom.window:dom.window[name];
globalThis.IS_REACT_ACT_ENVIRONMENT=true;
const React=require('react');
const {createRoot}=require('react-dom/client');
const {rolldown}=await import(pathToFileURL(require.resolve('rolldown')));
const temporary=await mkdtemp(fileURLToPath(new URL('../web/.practice-workspace-test-',import.meta.url)));
let bundle;
try {
 bundle=await rolldown({
  input:fileURLToPath(new URL('../web/src/PracticeWorkspace.tsx',import.meta.url)),
  external:id=>id==='react'||id.startsWith('react/'),
  tsconfig:fileURLToPath(new URL('../web/tsconfig.json',import.meta.url)),
 });
 await bundle.write({file:`${temporary}/workspace.mjs`,format:'es'});
 const {default:PracticeWorkspace}=await import(pathToFileURL(`${temporary}/workspace.mjs`));
 const deferred=()=>{let resolve,reject;const promise=new Promise((done,failed)=>{resolve=done;reject=failed;});return {promise,resolve,reject};};
 const snapshot=name=>({version:1,sequence:0,state:'ready',project:{
  version:4,name,track:{path:'source',audio:{encoding:'aac',sampleRate:48000,channels:2,frames:48000}},
  settings:{speed:1,pitch:0,loop:{enabled:false,start:0,end:1}},annotations:{},
 },capabilities:{formats:['aac','opus'],liveTransport:false,streamPreview:false}});
 const window={version:1,sha256:'fixture',sampleRate:48000,sourceFrames:48000,requestedStart:0,requestedEnd:48000,framesPerCell:48000,cells:[{startFrame:0,endFrame:48000,channels:[{min:-.2,max:.2},{min:-.2,max:.2}]}]};
 const button=text=>Array.from(document.querySelectorAll('button')).find(node=>node.textContent===text);
 for(const extension of ['m4a','opus'])for(const disposedResult of ['reject','resolve']) {
  const clients=[];
  const file=new File(['generated fixture'],`library.${extension}`);
  const createFacade=()=>{
   const opening=deferred(),waveform=deferred();
   const client={opening,openCalls:[],waveformCalls:[],disposals:0,
    open(source){this.openCalls.push(source);return opening.promise;},
    waveform(start,end){this.waveformCalls.push([start,end]);return waveform.promise;},
    exportProject:async()=>{throw new Error('Active project export failed.');},
    dispose(){this.disposals++;if(disposedResult==='reject'&&this.openCalls.length)opening.reject(new Error('Practice workspace closed.'));},
    cancel(){},
   };
   // Keep the deferred and facade method separate.
   client.waveformResult=waveform;
   clients.push(client);return client;
  };
  const root=createRoot(document.getElementById('root'));
  try {
   await React.act(async()=>root.render(React.createElement(React.StrictMode,null,
    React.createElement(PracticeWorkspace,{source:file,onClose(){},createFacade,createStorage:async()=>({list:async()=>[]})}))));
   assert.equal(clients.length,2,'Strict Mode did not replay workspace effects');
   const [disposed,active]=clients;
   assert.equal(disposed.disposals,1);
   assert.deepEqual(active.openCalls,[file],'library source did not reopen on the active facade');
   if(disposedResult==='resolve')await React.act(async()=>disposed.opening.resolve({snapshot:snapshot('stale track')}));
   assert.equal(document.querySelector('[role="alert"]'),null,'disposed facade failed the active workspace');
   assert.equal(button('Open song').disabled,true,'disposed completion unlocked the active import');
   assert.equal(disposed.waveformCalls.length,0,'disposed import started waveform loading');
   assert(!document.body.textContent.includes('stale track'),'disposed import replaced the active track');
   await React.act(async()=>active.opening.resolve({snapshot:snapshot(file.name)}));
   assert.deepEqual(active.waveformCalls,[[0,48000]]);
   assert.equal(button('Open song').disabled,true,'import ended before waveform loading');
   await React.act(async()=>active.waveformResult.resolve({window}));
   assert.equal(document.querySelector('[role="alert"]'),null);
   assert.equal(button('Open song').disabled,false);
   assert(document.body.textContent.includes(file.name));
   assert.equal(document.querySelector('[role="status"]').textContent,'Ready to practice. Select a phrase or press Play.');
   // Ownership guards must still surface real failures from the live facade.
   await React.act(async()=>button('Download project').click());
   assert.equal(document.querySelector('[role="alert"]').textContent,'Active project export failed.');
   assert.equal(document.querySelector('[role="status"]').textContent,'Operation could not finish.');
  } finally {await React.act(async()=>root.unmount());}
  assert(clients.every(client=>client.disposals===1),'workspace leaked or disposed a facade twice');
 }
 // Exercise continuous editing against a deliberately slow command port. The
 // accepted sequence must advance, while the newest speed/pitch remains editable.
 {
  let current=snapshot('Live editing');
  current.capabilities.streamPreview=true;
  const commands=[],waiting=[];
  const storeItems=[{id:'browser:saved',name:'Saved solo',sourceName:'solo.wav',savedAt:1}];
  const original=new File(['fixture'],'solo.wav');
  const client={liveSupported:true,openCalls:[],localCalls:[],
   async open(file){this.openCalls.push(file);current=snapshot(file.name);return {snapshot:structuredClone(current)};},
   async openLocal(file,id){this.localCalls.push([file,id]);current=snapshot('Saved solo');return {snapshot:structuredClone(current)};},
   async waveform(){return {window};},
   command(accepted,command){const result=deferred();commands.push({sequence:accepted.sequence,command});waiting.push(result);return result.promise;},
   cancel(){},dispose(){},
  };
  const root=createRoot(document.getElementById('root'));
  const input=name=>document.querySelector(`[aria-label="${name}"]`);
  const change=async(node,value)=>React.act(async()=>{
   // Use the native setter so React observes the simulated browser input.
   Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype,'value').set.call(node,String(value));
   node.dispatchEvent(new dom.window.Event('input',{bubbles:true}));
   node.dispatchEvent(new dom.window.Event('change',{bubbles:true}));
  });
  const flush=()=>React.act(async()=>new Promise(resolve=>setTimeout(resolve,85)));
  const accept=async()=>React.act(async()=>{
   const command=commands[commands.length-waiting.length].command;
   current=structuredClone(current);current.sequence++;
   if(command.type==='set_parameters')Object.assign(current.project.settings,{speed:command.speed,pitch:command.pitch});
   else if(command.type==='set_loop')current.project.settings.loop=command.loop;
   waiting.shift().resolve({snapshot:structuredClone(current)});
  });
  try {
   await React.act(async()=>root.render(React.createElement(PracticeWorkspace,{source:new File(['fixture'],'first.wav'),librarySources:[original],onClose(){},createFacade:()=>client,createStorage:async()=>({list:async()=>storeItems})})));
   assert(document.querySelector('aside[aria-label="Training projects"]'));
   assert(!document.body.textContent.includes('Apply speed / pitch'));
   await change(input('Speed slider'),65);await flush();
   assert.deepEqual(commands[0],{sequence:0,command:{type:'set_parameters',speed:.65,pitch:0}});
   assert.equal(input('Speed slider').disabled,false,'live command disabled continuous editing');
   await change(input('Speed slider'),75);
   await change(input('Pitch slider'),-2);
   assert.equal(commands.length,1,'overlapping command used a stale sequence');
   await accept();
   assert.deepEqual(commands[1],{sequence:1,command:{type:'set_parameters',speed:.75,pitch:-2}});
   await accept();
   assert.equal(input('Speed (%)').value,'75');assert.equal(input('Pitch (semitones)').value,'-2');
   // Handle edits automatically enable and validate the selection.
   const handle=document.querySelector('[aria-label="Selection start handle"]');
   await React.act(async()=>handle.dispatchEvent(new dom.window.KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true,cancelable:true})));
   await flush();assert.equal(commands[2].sequence,2);assert.equal(commands[2].command.type,'set_loop');assert.equal(commands[2].command.loop.enabled,true);
   assert.equal(Math.round(commands[2].command.loop.start*48000),1);await accept();
   // Invalid numbers stay in the input without being sent to the core.
   await change(input('Speed (%)'),0);await flush();assert.equal(commands.length,3);
   await change(input('Speed (%)'),80);await flush();
   await React.act(async()=>waiting.shift().resolve({snapshot:structuredClone(current),error:{code:'host_failure',message:'Device rejected settings'}}));
   assert.equal(input('Speed (%)').value,'75','rejected settings were shown as accepted');
   assert(document.querySelector('[role="alert"]').textContent.includes('Device rejected settings'));
   // Reopening a saved project chooses its matching library file rather than
   // trying the unrelated current song, and starts a fresh sequence.
   await React.act(async()=>document.querySelector('.saved-project-card').click());
   assert.deepEqual(client.localCalls,[[original,'browser:saved']]);
   await change(input('Speed slider'),90);await flush();assert.equal(commands.at(-1).sequence,0);await accept();
  } finally {await React.act(async()=>root.unmount());}
 }
 console.log(JSON.stringify({libraryAutoOpenSurvivesStrictMode:true,disposedRejectionIgnored:true,disposedSuccessIgnored:true,activeImportLatchPreserved:true,activeErrorsVisible:true,facadesDisposedOnce:true,continuousChangesCoalesced:true,latestSequenceUsed:true,loopChangesAutomatic:true,rejectedSettingsRestored:true,matchingLibraryOriginalReopened:true}));
} finally {await bundle?.close();await rm(temporary,{recursive:true,force:true});dom.window.close();}
