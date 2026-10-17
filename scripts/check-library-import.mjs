import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdtemp, rm, readFile} from 'node:fs/promises';
import {fileURLToPath, pathToFileURL} from 'node:url';
import {createHash} from 'node:crypto';
const require=createRequire(new URL('../web/package.json',import.meta.url));
const {rolldown}=await import(pathToFileURL(require.resolve('rolldown')));
const temporary=await mkdtemp(fileURLToPath(new URL('../web/.library-test-',import.meta.url)));
const bundles=[];
async function bundle(name,input,plugins=[]) {
 const build=await rolldown({input:fileURLToPath(new URL(input,import.meta.url)),external:id=>id==='react'||id.startsWith('react/'),tsconfig:fileURLToPath(new URL('../web/tsconfig.json',import.meta.url)),plugins});
 bundles.push(build);
 await build.write({file:`${temporary}/${name}.mjs`,format:'es',codeSplitting:false});
 return import(pathToFileURL(`${temporary}/${name}.mjs`));
}
try {
 // Exercise real parsing and persistent hash lookups in the worker module.
 globalThis.indexedDB=new (require('fake-indexeddb').IDBFactory)();
 const results=[];
 globalThis.self={postMessage:value=>results.push(value)};
 await bundle('worker','../web/src/library-worker.ts');
 const bytes=await readFile(new URL('../web/public/practice/demo.wav',import.meta.url));
 const file=new File([bytes],'original.wav',{lastModified:123});
 await self.onmessage({data:file});
 assert.equal(results.at(-1).tagError,undefined);
 assert.equal(results.at(-1).tags.format,'WAVE');
 const db=await new Promise((resolve,reject)=>{const request=indexedDB.open('riffine-tags-v1',1);request.onsuccess=()=>resolve(request.result);request.onerror=()=>reject(request.error);});
 const hash=createHash('sha256').update(bytes).digest('hex');
 const put=tags=>new Promise((resolve,reject)=>{const tx=db.transaction('tags','readwrite');tx.objectStore('tags').put({hash,tags,savedAt:Date.now()});tx.oncomplete=resolve;tx.onabort=()=>reject(tx.error);});
 await put({...results.at(-1).tags,title:'Cached title'});
 // Module owner and File objects both change, while IndexedDB persists.
 await bundle('fresh-worker','../web/src/library-worker.ts');
 await self.onmessage({data:new File([bytes],'renamed.wav',{lastModified:456})});
 assert.equal(results.at(-1).tags.title,'Cached title');
 const changed=Buffer.from(bytes);changed[changed.length-1]^=1;
 await self.onmessage({data:new File([changed],'original.wav',{lastModified:123})});
 assert.equal(results.at(-1).tags.title,undefined,'same-name/size/mtime replacement reused cached tags');
 await put({title:42});
 await self.onmessage({data:file});
 assert.equal(results.at(-1).tags.format,'WAVE','invalid cached metadata did not fall back');
 db.close();
 globalThis.indexedDB=undefined;
 await self.onmessage({data:file});
 assert.equal(results.at(-1).tags.format,'WAVE','unavailable cache prevented parsing');
 await self.onmessage({data:new File(['broken'],'broken.wav')});
 assert.equal(results.at(-1).tagError,true);

 const {JSDOM}=require('jsdom');
 const dom=new JSDOM('<div id="root"></div>',{url:'http://localhost/'});
 for(const name of ['window','document','Element','HTMLElement'])globalThis[name]=name==='window'?dom.window:dom.window[name];
 globalThis.IS_REACT_ACT_ENVIRONMENT=true;
 const readers=[];
 globalThis.Worker=class {
  constructor(){readers.push(this);}
  postMessage(file){this.file=file;}
  terminate(){this.terminated=true;}
 };
 const React=require('react'),{createRoot}=require('react-dom/client');
 const {default:App}=await bundle('app','../web/src/App.tsx',[{
  name:'test-player',
  resolveId(id){if(id==='./PracticeWorkspace'||id==='./OfflineStatus')return '\0'+id;},
  load(id){
   if(id==='\0./PracticeWorkspace')return 'import React from "react"; export default function Player({source}) {return React.createElement("p", {"data-testid":"player"}, source.name);}';
   if(id==='\0./OfflineStatus')return 'export default () => null';
  },
 }]);
 const root=createRoot(document.getElementById('root'));
 const choose=async files=>{
  const input=document.querySelector('input[type=file]');
  Object.defineProperty(input,'files',{configurable:true,value:files});
  await React.act(async()=>input.dispatchEvent(new dom.window.Event('change',{bubbles:true})));
 };
 await React.act(async()=>root.render(React.createElement(React.StrictMode,null,React.createElement(App))));
 const songs=Array.from({length:5},(_,i)=>new File(['audio'],`song-${i}.mp3`));
 await choose([...songs,new File(['ignored'],'notes.txt')]);
 assert.equal(document.querySelectorAll('.library-song').length,5,'folder rows waited for metadata');
 assert.equal(readers.length,2,'background concurrency must be bounded');
 assert(document.querySelector('[role=status]').textContent.includes('0 of 5'));
 assert.equal([...document.querySelectorAll('button')].find(button=>button.textContent==='Change folder').disabled,false);
 await React.act(async()=>document.querySelector('.library-song').click());
 assert.equal(document.querySelector('[data-testid=player]').textContent,'song-0.mp3','pending song was not playable');
 await React.act(async()=>readers[0].onmessage({data:{tags:{title:'Tagged song',artist:'Artist'}}}));
 assert.equal(document.querySelector('.song-title strong').textContent,'Tagged song');
 assert.equal(document.querySelectorAll('.library-song')[1].textContent.includes('song-1'),true,'rows reordered during import');
 const late=readers[1].onmessage;
 await choose([new File(['new'],'replacement.flac')]);
 assert(readers[0].terminated&&readers[1].terminated,'changing folders left old readers running');
 await React.act(async()=>late({data:{tags:{title:'Stale song'}}}));
 assert.equal(document.querySelectorAll('.library-song').length,1);
 assert.equal(document.querySelector('.song-title strong').textContent,'replacement');
 await React.act(async()=>readers.at(-1).onmessage({data:{tagError:true}}));
 assert.equal(document.querySelector('.song-title small').textContent,'Unknown artist');
 assert(document.querySelector('[role=status]').textContent.includes('1 songs ready'));
 await React.act(async()=>root.unmount());
 assert(readers.at(-1).terminated,'finished reader was not released');
 console.log('Library: immediate playable rows, stable ordering, background progress, folder cancellation, tag failures, persistent content cache and storage fallback passed');
} finally {
 for(const build of bundles)await build.close();
 await rm(temporary,{recursive:true,force:true});
}
