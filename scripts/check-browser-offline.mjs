import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
const generated=await readFile(new URL('../web/dist/offline-sw.js',import.meta.url),'utf8');
const base=JSON.parse(generated.match(/^const BASE = (.*);$/m)[1]);
const pathFor=file=>base+file;
const assets=JSON.parse(generated.match(/^const ASSETS = (.*);$/m)[1]);
for(const file of ['index.html','practice/core.wasm','practice/dsp.mjs','practice/riffine-browser-dsp.wasm','practice/media/libav-4.10.6.1.1-fat.wasm.wasm','practice/project-store.mjs'])assert(assets.includes(pathFor(file)),file);
assert(assets.every(path=>path.startsWith(base)),'assets escaped deployment scope');
assert(!assets.some(path=>path.includes('/sources/')),'source archives should remain packaged, not precached');
const previous=`riffine-shell-${encodeURIComponent(base)}-previous`;
const data=new Map([[previous,new Map([[pathFor('index.html'),new Response('previous')]])],['riffine-shell-other-app-previous',new Map()]]);
const caches={
 async open(name){if(!data.has(name))data.set(name,new Map());const entries=data.get(name);return {put:async(path,response)=>entries.set(path,response.clone()),match:async(path)=>entries.get(path)?.clone()};},
 keys:async()=>[...data.keys()],delete:async(name)=>data.delete(name),
};
let online=true,broken,networkCalls=0,claims=0;
const handlers={};
const scope={
 addEventListener(type,handler){handlers[type]=handler;},
 location:{origin:'http://localhost'},clients:{claim:async()=>{claims++;}},
};
const context=vm.createContext({self:scope,caches,URL,Headers,Response,Request:class{constructor(path,options){this.url='http://localhost'+path;this.options=options;}},fetch:async request=>{
 networkCalls++;if(!online)throw new Error('Network unavailable');
 const path=new URL(request.url).pathname;
 // Model GitHub Pages: no isolation headers are sent by the static host.
 return new Response(path,{status:path===broken?404:200,headers:{'Content-Type':path===pathFor('index.html')?'text/html':'application/octet-stream'}});
}});
vm.runInContext(generated,context);
const dispatch=async(type,fields={})=>{let result;handlers[type]({...fields,waitUntil:promise=>{result=promise;},respondWith:promise=>{result=promise;}});return result;};
broken=pathFor('practice/core.wasm');
await assert.rejects(dispatch('install'),/Offline asset unavailable/);
assert(data.has(previous),'failed update deleted the last valid shell');
assert.equal(data.size,2,'failed update left a partial cache');
broken=undefined;await dispatch('install');
assert(data.has(previous),'installation prematurely deleted active assets');
await dispatch('activate');assert.equal(claims,1);assert(data.has('riffine-shell-other-app-previous'));assert(!data.has(previous));
online=false;const before=networkCalls;
for(const path of assets){const response=await dispatch('fetch',{request:{method:'GET',mode:path===pathFor('index.html')?'navigate':'cors',url:'http://localhost'+path}});assert.equal(await response.text(),path);assert.equal(response.headers.get('Cross-Origin-Embedder-Policy'),'require-corp');}
assert.equal(networkCalls,before,'cached practice requested network');
const page=await dispatch('fetch',{request:{method:'GET',mode:'navigate',url:'http://localhost'+base}});
assert.equal(page.headers.get('Cross-Origin-Opener-Policy'),'same-origin');assert.equal(page.headers.get('Cross-Origin-Embedder-Policy'),'require-corp');
assert.equal(await dispatch('fetch',{request:{method:'POST',url:'http://localhost/private-audio'}}),undefined);
assert.equal(await dispatch('fetch',{request:{method:'GET',mode:'cors',url:'http://localhost/private-audio'}}),undefined);
assert.equal(await dispatch('fetch',{request:{method:'GET',mode:'cors',url:'https://example.com/audio.wav'}}),undefined);
if(base!=='/')assert.equal(await dispatch('fetch',{request:{method:'GET',mode:'navigate',url:'http://localhost/other-app/'}}),undefined);
// Storage eviction still gets isolation headers from a plain static server.
online=true;
for(const [name,entries] of data)if(name!==previous)entries.delete(pathFor('practice/worker.js'));
const worker=await dispatch('fetch',{request:{method:'GET',mode:'cors',url:'http://localhost'+pathFor('practice/worker.js')}});
assert.equal(worker.headers.get('Cross-Origin-Embedder-Policy'),'require-corp');
console.log(JSON.stringify({completeOfflineShell:true,base,assets:assets.length,failedInstallPreservesActiveVersion:true,noPartialCache:true,offlineWASMAndWorkerFetch:true,staticHostIsolationHeaders:true,otherAppCachesPreserved:true,userResourcesNotCached:true}));
