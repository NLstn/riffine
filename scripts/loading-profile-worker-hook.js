// Development-only instrumentation, prepended by the loading profile server.
// Production worker assets are read without modifying their source files.
(() => {
 const stats={fileReads:0,fileReadBytes:0,fileReadMs:0,shortTimers:0,shortTimerRequestedMs:0,shortTimerElapsedMs:0,codecOpens:0,codecOpenMs:0,codecReads:0,codecReadFrames:0,codecReadMs:0,codecSeeks:0,codecSeekMs:0};
 const timeout=self.setTimeout.bind(self);
 self.setTimeout=(callback,ms,...args)=>{
  if(typeof callback!=='function'||ms>3)return timeout(callback,ms,...args);
  const start=performance.now();
  return timeout(()=>{stats.shortTimers++;stats.shortTimerRequestedMs+=ms||0;stats.shortTimerElapsedMs+=performance.now()-start;callback(...args);},ms);
 };
 const read=FileReaderSync.prototype.readAsArrayBuffer;
 FileReaderSync.prototype.readAsArrayBuffer=function(blob){
  const started=performance.now();try{return read.call(this,blob);}
  finally{stats.fileReads++;stats.fileReadBytes+=blob.size;stats.fileReadMs+=performance.now()-started;}
 };
 let openCodec;
 Object.defineProperty(self,'riffineCodecOpen',{configurable:true,get:()=>openCodec,set:open=>{
  openCodec=(file,codec,complete,failed)=>{
   const started=performance.now();stats.codecOpens++;
   return open(file,codec,value=>{
    stats.codecOpenMs+=performance.now()-started;
    const call=value.port.call;
    value.port.call=(method,args,done,reject)=>{
     const began=performance.now();
     return call.call(value.port,method,args,result=>{
      const elapsed=performance.now()-began;
      if(method==='read'){stats.codecReads++;stats.codecReadFrames+=result.frames;stats.codecReadMs+=elapsed;}
      if(method==='seek'){stats.codecSeeks++;stats.codecSeekMs+=elapsed;}
      done(result);
     },reject);
    };
    complete(value);
   },failed);
  };
 }});
 const post=self.postMessage.bind(self);
 self.postMessage=(message,...rest)=>{
  if(message?.type==='result'||message?.type==='ready')message={...message,loadingProfile:{...stats}};
  return post(message,...rest);
 };
})();
