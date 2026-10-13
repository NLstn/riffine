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
    React.createElement(PracticeWorkspace,{source:file,onClose(){},createFacade}))));
   assert.equal(clients.length,2,'Strict Mode did not replay workspace effects');
   const [disposed,active]=clients;
   assert.equal(disposed.disposals,1);
   assert.deepEqual(active.openCalls,[file],'library source did not reopen on the active facade');
   if(disposedResult==='resolve')await React.act(async()=>disposed.opening.resolve({snapshot:snapshot('stale track')}));
   assert.equal(document.querySelector('[role="alert"]'),null,'disposed facade failed the active workspace');
   assert.equal(button('Choose audio / relink').disabled,true,'disposed completion unlocked the active import');
   assert.equal(disposed.waveformCalls.length,0,'disposed import started waveform loading');
   assert(!document.body.textContent.includes('stale track'),'disposed import replaced the active track');
   await React.act(async()=>active.opening.resolve({snapshot:snapshot(file.name)}));
   assert.deepEqual(active.waveformCalls,[[0,48000]]);
   assert.equal(button('Choose audio / relink').disabled,true,'import ended before waveform loading');
   await React.act(async()=>active.waveformResult.resolve({window}));
   assert.equal(document.querySelector('[role="alert"]'),null);
   assert.equal(button('Choose audio / relink').disabled,false);
   assert(document.body.textContent.includes(file.name));
   assert.equal(document.querySelector('[role="status"]').textContent,'Original fingerprinted. Practice changes stay local.');
   // Ownership guards must still surface real failures from the live facade.
   await React.act(async()=>button('Download project').click());
   assert.equal(document.querySelector('[role="alert"]').textContent,'Active project export failed.');
   assert.equal(document.querySelector('[role="status"]').textContent,'Operation could not finish.');
  } finally {await React.act(async()=>root.unmount());}
  assert(clients.every(client=>client.disposals===1),'workspace leaked or disposed a facade twice');
 }
 console.log(JSON.stringify({libraryAutoOpenSurvivesStrictMode:true,disposedRejectionIgnored:true,disposedSuccessIgnored:true,activeImportLatchPreserved:true,activeErrorsVisible:true,facadesDisposedOnce:true}));
} finally {await bundle?.close();await rm(temporary,{recursive:true,force:true});dom.window.close();}
