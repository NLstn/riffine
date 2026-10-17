// RPC lives on the resource worker; terminating the decoder also cancels a long initial scan.
let WorkerClass=globalThis.Worker;
if(typeof process==='object' && process.versions?.node)WorkerClass=(await import('node:worker_threads')).Worker;
// Duration comes from this immutable File or from the core's content-verified
// persistent inspection. Never use filename/size or saved project metadata.
const inspections=new WeakMap();
export function openCodec(file,codec,complete,failed,verifiedInfo) {
 let worker;
 try {worker=new WorkerClass(new URL('./codec-worker.mjs',import.meta.url),{type:'module'});}
 catch(error){failed(String(error));return fence=>fence('');}
 let next=0,closed=false,termination,pending=new Map();
 const receive=message=> {
  const call=pending.get(message.id);if(!call)return;
  pending.delete(message.id);message.error ? call.reject(new Error(message.error)) : call.resolve(message.result);
 };
 const fail=error=>{for(const call of pending.values())call.reject(error);pending.clear();};
 const crashed=error=>{fail(error);void close();};
 if(worker.on){worker.on('message',receive);worker.on('error',crashed);worker.on('exit',()=>crashed(new Error('Decoder worker closed.')));}
 else {worker.onmessage=event=>receive(event.data);worker.onerror=()=>crashed(new Error('Decoder worker failed.'));}
 function close() {
  if(!closed){closed=true;termination=Promise.resolve(worker.terminate());fail(new Error('cancelled: audio decoding stopped'));}
  return termination;
 }
 function request(method,args={}) {
  if(closed)return Promise.reject(new Error('Decoder closed.'));
  return new Promise((resolve,reject)=>{const id=++next;pending.set(id,{resolve,reject});try{worker.postMessage({id,method,...args});}catch(error){pending.delete(id);reject(error);}});
 }
 function invoke(method,args,done,reject) {
  const result=method==='close' ? close().then(()=>({})) : request(method,args);
  void result.then(done,error=>{void close().then(()=>reject(String(error)));});
  return fence=>{void close().then(()=>fence(''),error=>fence(String(error)));};
 }
 const port={call:invoke};
 const cached=inspections.get(file);
 const result=request('open',{file,codec,knownInfo:cached?.codec===codec ? cached.info : verifiedInfo});
 void result.then(info=>{
  if(closed)return;
  inspections.set(file,{codec,info:{...info}});
  complete({info,port});
 },error=>{void close().then(()=>failed(String(error)));});
 return fence=>{void close().then(()=>fence(''),error=>fence(String(error)));};
}
