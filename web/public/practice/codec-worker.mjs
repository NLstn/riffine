// One borrowed immutable File, one decoder cursor, fixed WASM memory and PCM blocks.
let send = value => self.postMessage(value), fileReader;
if (typeof process === 'object' && process.versions?.node) {
 const {parentPort} = await import('node:worker_threads');
 send = value => parentPort.postMessage(value);
 parentPort.on('message', message => void receive(message));
} else {
 self.onmessage = event => void receive(event.data);
 fileReader = new FileReaderSync();
}
let module, media, decoder=0, buffer=0, channels=0, frame=0, frames=0, opened=false;
async function close() { if(media)await media.close(); media=null; if(decoder)module._riffine_browser_decoder_destroy(decoder); if(buffer)module._free(buffer); decoder=buffer=0; }
async function receive({id,method,file,codec,knownInfo,count,position}) {
 try {
  if(method==='open') {
   if(opened)throw new Error('Decoder already opened.'); opened=true;
   if(!Number.isSafeInteger(file.size)||file.size<1||file.size>2**48||![1,2,3,4].includes(codec))throw new Error('Invalid compressed source.');
   const readFile=(offset,size)=> {
    if(!Number.isSafeInteger(offset)||offset<0||size<1||size>65536||offset+size>file.size)throw new Error('Invalid compressed source read.');
    // Node tests supply a structured clone of the controlled file; browsers use FileReaderSync.
    if(file.bytes instanceof Uint8Array)return file.bytes.subarray(offset,offset+size);
    const bytes=new Uint8Array(fileReader.readAsArrayBuffer(file.slice(offset,offset+size)));
    if(bytes.length!==size)throw new Error('Source read was truncated.');return bytes;
   };
   if(codec===3||codec===4) {
    const {MediaDecoder}=await import('./media-decoder.mjs');
    media=await MediaDecoder.open({size:file.size,read:readFile},codec,knownInfo);
    channels=media.info.channels;frames=media.info.frames;
    send({id,result:media.info});return;
   }
   const {default:createModule}=await import('./dsp.mjs');
   module=await createModule();module.riffineReadFile=readFile;
   decoder=module._riffine_browser_decoder_create(codec,BigInt(file.size));
   if(!decoder)throw new Error('Cannot decode this audio. Supported practice formats: PCM WAV, MP3, FLAC, M4A and Opus (mono/stereo, 8–192 kHz).');
   channels=module._riffine_browser_decoder_channels(decoder);
   frames=Number(module._riffine_browser_decoder_frames(decoder));
   buffer=module._malloc(8192*channels*4);if(!buffer)throw new Error('Decoder buffer allocation failed.');
   send({id,result:{encoding:codec===1?'mp3':'flac',channels,sampleRate:module._riffine_browser_decoder_rate(decoder),frames,bits:0}});
  } else if(method==='read') {
   if((!decoder&&!media)||!Number.isInteger(count)||count<1||count>8192)throw new Error('Invalid PCM read.');
   if(media) {const result=await media.read(count);frame+=result.frames;send({id,result});return;}
   const n=module._riffine_browser_decoder_read(decoder,buffer,count);
   if(n<0||n>count||frame+n>frames)throw new Error('Compressed audio is corrupt or truncated.');
   frame+=n;
   send({id,result:{frames:n,pcm:module.HEAPU8.slice(buffer,buffer+n*channels*4)}});
  } else if(method==='seek') {
   if((!decoder&&!media)||!Number.isSafeInteger(position)||position<0||position>frames)throw new Error('Invalid source seek.');
   if(media) {await media.seek(position);frame=position;send({id,result:{}});return;}
   if(position!==frame && module._riffine_browser_decoder_seek(decoder,BigInt(position)))throw new Error('Cannot seek this audio.');
   frame=position;send({id,result:{}});
  } else throw new Error('Invalid decoder operation.');
 } catch(error) {await close();send({id,error:String(error)});}
}
