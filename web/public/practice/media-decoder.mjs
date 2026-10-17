// M4A and Ogg Opus cursor, owned by the cancellable codec worker.
// Reads original File slices; retains one compressed packet and one PCM block.
export class MediaDecoder {
 static async open(source, codec, knownInfo) {
  const [{default: LibAV}, {default: factory}] = await (codec===3 ? Promise.all([
   import('./media/libav.mjs'), import('./media/factory.mjs'),
  ]) : Promise.all([
   import('./media/opus-libav.mjs'), import('./media/opus-factory.mjs'),
  ]));
  const boundedFactory = () => codec===3
   ? factory({wasmMemory: new WebAssembly.Memory({initial:512, maximum:1024})})
   : factory(); // 6.x allocator limit is applied when packaging the module.
  LibAV.LibAVFactory = boundedFactory; // 4.x factory override
  const av = await LibAV.LibAV({noworker:true, nosimd:true, nothreads:true, factory:boundedFactory});
  const cursor = new MediaDecoder(av, codec);
  try {
   if (knownInfo) {
    if (!Number.isSafeInteger(knownInfo.frames) || knownInfo.frames<1 || knownInfo.frames>2**48 ||
        !Number.isInteger(knownInfo.channels) || knownInfo.channels<1 || knownInfo.channels>2 ||
        !Number.isInteger(knownInfo.sampleRate) || knownInfo.sampleRate<8000 || knownInfo.sampleRate>192000 ||
        (codec===3 ? !['aac','alac'].includes(knownInfo.encoding) : knownInfo.encoding!=='opus')) throw new Error('Invalid inspected audio dimensions.');
    cursor.info={...knownInfo};
   }
   await av.av_log_set_level(av.AV_LOG_ERROR);
   av.onblockread = async (name, position, length) => {
    try {
     if (!Number.isSafeInteger(position) || position < 0 || position > source.size || !Number.isSafeInteger(length) || length < 1) throw new Error('Invalid media source read.');
     const count = Math.min(length, 65536, source.size-position);
     const bytes = count ? source.read(position, count) : new Uint8Array();
     await av.ff_block_reader_dev_send(name, position, bytes);
    } catch (error) {
     await av.ff_block_reader_dev_send(name, position, null, {error});
    }
   };
   await av.mkblockreaderdev(cursor.name, source.size);
   await cursor.reset();
   if (knownInfo) {
    if (cursor.encoding!==knownInfo.encoding) throw new Error('Inspected audio encoding changed.');
    return cursor;
   }
   // Count actual decoded samples (including codec delay/padding rules), without
   // retaining the song. Metadata alone is not an exact source-frame clock.
   let frames = 0, block;
   while ((block = await cursor.next()) !== null) {
    frames += block.length/cursor.channels;
    if (!Number.isSafeInteger(frames) || frames > 2**48) throw new Error('Audio exceeds the source frame limit.');
   }
   if (!frames) throw new Error('Audio contains no decoded frames.');
   cursor.info = {encoding:cursor.encoding, channels:cursor.channels, sampleRate:cursor.rate, frames, bits:0};
   await cursor.reset();
   return cursor;
  } catch (error) {
   await cursor.close();
   throw error;
  }
 }
 constructor(av, codec) {
  this.av=av; this.codec=codec; this.name=codec===3?'source.m4a':'source.opus';
  this.format=0; this.context=0; this.packet=0; this.frame=0;
  this.pending=null; this.offset=0; this.position=0;
 }
 async freeCursor() {
  if (this.context) await this.av.ff_free_decoder(this.context, this.packet, this.frame);
  if (this.format) await this.av.avformat_close_input_js(this.format);
  this.context=this.packet=this.frame=this.format=0;
 }
 async reset() {
  await this.freeCursor();
  const av=this.av;
  const [format, streams]=await av.ff_init_demuxer_file(this.name, this.codec===3?'mov':'ogg');
  this.format=format;
  const audio=streams.filter(stream=>stream.codec_type===av.AVMEDIA_TYPE_AUDIO);
  if (audio.length!==1) throw new Error('Choose a file containing one audio track.');
  this.stream=audio[0];
  const encoding=await av.avcodec_get_name(this.stream.codec_id);
  if (this.codec===3 ? !['aac','alac'].includes(encoding) : encoding!=='opus') throw new Error('Supported media: M4A with AAC or Apple Lossless, and Ogg Opus.');
  this.encoding=encoding;
  for (const stream of streams) if (stream.index!==this.stream.index) await av.AVStream_discard_s(stream.ptr, av.AVDISCARD_ALL);
  const [,context,packet,frame]=await av.ff_init_decoder(this.stream.codec_id, this.stream.codecpar);
  this.context=context; this.packet=packet; this.frame=frame;
  this.channels=this.info?.channels??await av.AVCodecContext_channels(context);
  this.rate=this.info?.sampleRate??await av.AVCodecContext_sample_rate(context);
  if (this.channels<1 || this.channels>2 || this.rate<8000 || this.rate>192000) throw new Error('Supported audio dimensions: mono/stereo, 8–192 kHz.');
  // AAC's final packet may contain padding beyond the MP4 edit duration.
  // Opus's duration includes pre-skip; libopus applies its own skip side data.
  this.end=this.codec===3 && this.stream.duration>0 ? Math.round(this.stream.duration*this.rate) : Infinity;
  this.dimensionsKnown=!!this.info;
  this.eof=false; this.pending=null; this.offset=0; this.position=0; this.decoded=0;
 }
 async next() {
  const av=this.av;
  while (!this.eof) {
   const [status, packets]=await (this.codec===3
    ? av.ff_read_multi(this.format, this.packet, undefined, {limit:1})
    : av.ff_read_frame_multi(this.format, this.packet, {limit:1}));
   if (status!==-av.EAGAIN && status!==av.AVERROR_EOF) throw new Error('Media is corrupt or truncated.');
   this.eof=status===av.AVERROR_EOF;
   const input=packets[this.stream.index]??[];
   if (input.some(packet=>packet.data.length>65536 || packet.flags&av.AV_PKT_FLAG_CORRUPT)) throw new Error('Invalid compressed audio packet.');
   const frames=await av.ff_decode_multi(this.context, this.packet, this.frame, input, {fin:this.eof, ignoreErrors:false});
   let samples=0;
   for (const frame of frames) {
    if (!this.dimensionsKnown) {
     // HE-AAC can reveal its full rate/channel layout only on first decode.
     this.channels=frame.channels; this.rate=frame.sample_rate;
     if (this.channels<1 || this.channels>2 || this.rate<8000 || this.rate>192000) throw new Error('Supported audio dimensions: mono/stereo, 8–192 kHz.');
     this.end=this.codec===3 && this.stream.duration>0 ? Math.round(this.stream.duration*this.rate) : Infinity;
     this.dimensionsKnown=true;
    }
    if (frame.channels!==this.channels || frame.sample_rate!==this.rate || !Number.isInteger(frame.nb_samples) || frame.nb_samples<0) throw new Error('Decoded audio dimensions changed.');
    samples+=frame.nb_samples;
   }
   if (samples>8192) throw new Error('Decoded audio block exceeds 8192 frames.');
   const count=Math.min(samples, Math.max(0, this.end-this.decoded));
   const pcm=new Float32Array(count*this.channels);
   let output=0;
   for (const frame of frames) {
    const planar=frame.format>=av.AV_SAMPLE_FMT_U8P;
    const format=planar ? frame.format-av.AV_SAMPLE_FMT_U8P : frame.format;
    for (let i=0; i<frame.nb_samples && output<count; i++,output++) for (let c=0; c<this.channels; c++) {
     let value=planar ? frame.data[c][i] : frame.data[i*this.channels+c];
     if (format===av.AV_SAMPLE_FMT_U8) value=(value-128)/128;
     else if (format===av.AV_SAMPLE_FMT_S16) value/=32768;
     else if (format===av.AV_SAMPLE_FMT_S32) value/=2147483648;
     else if (format!==av.AV_SAMPLE_FMT_FLT && format!==av.AV_SAMPLE_FMT_DBL) throw new Error('Unsupported decoded sample format.');
     if (!Number.isFinite(value)) throw new Error('Non-finite decoded audio.');
     pcm[output*this.channels+c]=value;
    }
   }
   this.decoded+=count;
   if (count) return pcm;
  }
  return null;
 }
 async read(count) {
  const output=new Float32Array(count*this.channels);
  let frames=0;
  while (frames<count) {
   if (!this.pending || this.offset===this.pending.length) {
    this.pending=await this.next(); this.offset=0;
    if (!this.pending) break;
   }
   const samples=Math.min((count-frames)*this.channels, this.pending.length-this.offset);
   output.set(this.pending.subarray(this.offset,this.offset+samples),frames*this.channels);
   this.offset+=samples; frames+=samples/this.channels;
  }
  this.position+=frames;
  return {frames, pcm:new Uint8Array(output.buffer,0,frames*this.channels*4)};
 }
 async seek(position) {
  // Replay ensures identical decoder state and pre-skip at every loop boundary.
  if (position<this.position) await this.reset();
  while (this.position<position) {
   const count=Math.min(8192,position-this.position);
   if ((await this.read(count)).frames!==count) throw new Error('Media is truncated during seek.');
  }
 }
 async close() {await this.freeCursor(); this.pending=null; this.av.terminate();}
}
