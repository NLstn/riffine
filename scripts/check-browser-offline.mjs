import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';
import {setImmediate as tick} from 'node:timers/promises';
const generated=await readFile(new URL('../web/dist/offline-sw.js',import.meta.url),'utf8');
const constant=name=>JSON.parse(generated.match(new RegExp(`^const ${name} = (.*);$`,'m'))[1]);
const base=constant('BASE'), assets=constant('ASSETS'), shell=constant('SHELL'), hashes=constant('HASHES');
const pathFor=file=>base+file;
for(const file of ['index.html','practice/core.wasm','practice/dsp.mjs','practice/riffine-browser-dsp.wasm','practice/media/libav-4.10.6.1.1-fat.wasm.wasm','practice/project-store.mjs'])assert(assets.includes(pathFor(file)),file);
assert(assets.every(path=>path.startsWith(base)),'assets escaped deployment scope');
assert(!assets.some(path=>path.includes('/sources/')||path.includes('/licenses/')||path.endsWith('LICENSES.txt')||path.endsWith('SOURCES.txt')),'notices/sources must not gate startup or offline preparation');
assert((await readFile(new URL('../web/dist/practice/licenses/THIRD_PARTY_NOTICES.md',import.meta.url),'utf8')).length,'notices must remain packaged');
assert(shell.includes(pathFor('index.html')) && shell.length<=3,'startup must have a small shell');
assert(!shell.some(path=>path.includes('/practice/')),'audio engines must not gate worker activation');
const prefix=`riffine-shell-${encodeURIComponent(base)}-`,previous=prefix+'previous';
function runtime(source=generated,data=new Map([[previous,new Map([[pathFor('index.html'),new Response('previous')]])],['riffine-shell-other-app-previous',new Map()]])) {
 const state={data,online:true,broken:undefined,calls:[],active:0,maxActive:0,claims:0};
 const caches={
  async open(name){if(!data.has(name))data.set(name,new Map());const entries=data.get(name);return {put:async(path,response)=>entries.set(path,response.clone()),match:async(path)=>entries.get(path)?.clone()};},
  keys:async()=>[...data.keys()],delete:async(name)=>data.delete(name),
 };
 const handlers={};
 const scope={addEventListener(type,handler){handlers[type]=handler;},location:{origin:'http://localhost'},clients:{claim:async()=>{state.claims++;}}};
 vm.runInContext(source,vm.createContext({self:scope,caches,URL,Headers,Response,Request:class{constructor(path,options){this.url='http://localhost'+path;this.options=options;}},fetch:async request=>{
  const url=new URL(request.url),path=url.pathname;
  state.calls.push(path);assert(url.searchParams.get('v'),'asset requests need a content version');
  if(!state.online)throw new Error('Network unavailable');
  state.maxActive=Math.max(state.maxActive,++state.active);await tick();state.active--;
  return new Response(path,{status:path===state.broken?404:200,headers:{'Content-Type':path===pathFor('index.html')?'text/html':'application/octet-stream'}});
 }}));
 state.dispatch=async(type,fields={})=>{let result;handlers[type]({...fields,waitUntil:promise=>{result=promise;},respondWith:promise=>{result=promise;}});return result;};
 state.fetch=path=>state.dispatch('fetch',{request:{method:'GET',mode:path===base?'navigate':'cors',url:'http://localhost'+path}});
 state.message=async type=>{const replies=[];await state.dispatch('message',{data:{type},ports:[{postMessage:value=>replies.push(value)}]});return replies;};
 return state;
}
const failed=runtime();failed.broken=pathFor('index.html');
await assert.rejects(failed.dispatch('install'),/Offline asset unavailable/);
assert(failed.data.has(previous));assert.equal(failed.data.size,2,'failed install left partial writes');
const app=runtime();await app.dispatch('install');
assert.deepEqual([...app.calls].sort(),[...shell].sort());
assert(app.maxActive>1 && app.maxActive<=6,'shell downloads were not bounded/parallel');
await app.dispatch('activate');assert.equal(app.claims,1);assert(app.data.has(previous),'activation deleted the last complete package');
// Concurrent on-demand requests share one fetch and write it through to cache.
const worker=pathFor('practice/worker.js'),beforeWorker=app.calls.length;
const responses=await Promise.all([app.fetch(worker),app.fetch(worker)]);
assert.equal(app.calls.length-beforeWorker,1);
for(const response of responses){assert.equal(await response.text(),worker);assert.equal(response.headers.get('Cross-Origin-Embedder-Policy'),'require-corp');}
await app.fetch(worker);assert.equal(app.calls.length-beforeWorker,1,'cache miss was not persisted');
app.broken=pathFor('practice/core.wasm');
const failure=await app.message('prepare_offline');assert.equal(failure.at(-1).state,'failed');assert(app.data.has(previous));
assert.equal((await app.message('offline_status'))[0].complete,false,'partial package reported offline ready');
app.broken=undefined;
assert.equal((await app.message('prepare_offline')).at(-1).state,'ready');
assert.equal(app.maxActive,6,'offline downloads did not run six at a time');
assert(!app.data.has(previous));assert(app.data.has('riffine-shell-other-app-previous'));
app.online=false;const before=app.calls.length;
for(const path of assets){const response=await app.fetch(path);assert.equal(await response.text(),path);assert.equal(response.headers.get('Cross-Origin-Embedder-Policy'),'require-corp');}
const page=await app.fetch(base);assert.equal(page.headers.get('Cross-Origin-Opener-Policy'),'same-origin');
assert.equal((await app.message('offline_status'))[0].complete,true);
assert.equal((await app.message('prepare_offline')).at(-1).state,'ready');
assert.equal(app.calls.length,before,'repeat visits/offline preparation redownloaded cached assets');
await app.message('claim');assert.equal(app.claims,2,'uncontrolled reload recovery did not claim clients');
for(const request of [{method:'POST',url:'http://localhost'+worker},{method:'GET',url:'http://localhost'+pathFor('private-audio')},{method:'GET',url:'https://example.com/audio.wav'}])assert.equal(await app.dispatch('fetch',{request}),undefined);
if(base!=='/')assert.equal(await app.fetch('/other-app/'),undefined);
// Replacing one asset reuses the other version's unchanged content, while
// retaining that previous package until the replacement is fully prepared.
const changed=pathFor('practice/dsp.mjs'),nextHashes={...hashes,[changed]:'new-dsp-content'};
const nextSource=generated.replace(/^const VERSION = .*;$/m,'const VERSION = "next";').replace(/^const HASHES = .*;$/m,`const HASHES = ${JSON.stringify(nextHashes)};`);
const next=runtime(nextSource,app.data),oldCache=[...app.data.keys()].find(name=>name.startsWith(prefix));
await next.dispatch('install');assert.equal(next.calls.length,0,'unchanged startup assets were downloaded again');
await next.dispatch('activate');assert(next.data.has(oldCache));
assert.equal((await next.message('prepare_offline')).at(-1).state,'ready');
assert.deepEqual(next.calls,[changed],'unchanged assets were downloaded again during update');
assert(!next.data.has(oldCache));assert(next.data.has('riffine-shell-other-app-previous'));
// A later eviction also repairs the cache instead of refetching on every reload.
for(const entries of next.data.values())entries.delete(worker);
const repair=next.calls.length;await next.fetch(worker);await next.fetch(worker);
assert.equal(next.calls.length-repair,1);
console.log(JSON.stringify({base,startupAssets:shell.length,offlineAssets:assets.length,parallelDownloads:app.maxActive,activationBeforeEngineDownloads:true,failedPreparationPreservesPrevious:true,repeatVisitsUseCache:true,evictionRepaired:true,unchangedUpdateAssetsReused:true,noticesPackagedWithoutPrecache:true,isolationHeaders:true}));
