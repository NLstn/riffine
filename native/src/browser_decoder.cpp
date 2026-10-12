// Compressed source decoding runs on a dedicated resource worker, never an audio callback.
#include <emscripten.h>
#include "miniaudio.h"
#include <algorithm>
#include <array>
#include <cmath>
#include <cstdint>
#include <memory>

EM_JS(int, read_file_block, (double offset, uint32_t destination, uint32_t count), {
 try {
  const bytes = Module.riffineReadFile(offset, count);
  HEAPU8.set(bytes, destination);
  return bytes.length;
 } catch (_) { return -1; }
});
struct BrowserDecoder {
 ma_decoder decoder{};
 uint64_t size=0, position=0, frames=0;
 uint32_t channels=0, rate=0, codec=0;
 std::array<float,8192*2> scratch{};
 bool initialized=false;
 ~BrowserDecoder(){ if(initialized)ma_decoder_uninit(&decoder); }
 static ma_result read(ma_decoder *d,void *dst,size_t count,size_t *readBytes){
  auto &s=*static_cast<BrowserDecoder*>(d->pUserData);*readBytes=0;
  if(count && s.position==s.size)return MA_AT_END;
  count=size_t(std::min<uint64_t>(count,s.size-s.position));
  while(*readBytes<count){
   auto n=read_file_block(double(s.position),uint32_t(reinterpret_cast<uintptr_t>(dst)+*readBytes),uint32_t(std::min<size_t>(65536,count-*readBytes)));
   if(n<=0 || size_t(n)>count-*readBytes)return MA_IO_ERROR;
   s.position+=uint32_t(n);*readBytes+=uint32_t(n);
  }
  return MA_SUCCESS;
 }
 static ma_result seek(ma_decoder *d,ma_int64 offset,ma_seek_origin origin){
  auto &s=*static_cast<BrowserDecoder*>(d->pUserData);
  int64_t base=origin==ma_seek_origin_start ? 0 : int64_t(s.position);
  if(offset < -base || offset > int64_t(s.size)-base)return MA_BAD_SEEK;
  s.position=uint64_t(base+offset);return MA_SUCCESS;
 }
};
extern "C" {
uintptr_t riffine_browser_decoder_create(uint32_t codec,uint64_t size){
 if((codec!=1 && codec!=2)||!size||size>(uint64_t(1)<<48))return 0;
 auto s=std::make_unique<BrowserDecoder>();s->size=size;s->codec=codec;
 auto config=ma_decoder_config_init(ma_format_f32,0,0);
 config.encodingFormat=codec==1 ? ma_encoding_format_mp3 : ma_encoding_format_flac;
 if(ma_decoder_init(BrowserDecoder::read,BrowserDecoder::seek,s.get(),&config,&s->decoder)!=MA_SUCCESS)return 0;
 s->initialized=true;
 ma_format format;
 if(ma_decoder_get_data_format(&s->decoder,&format,&s->channels,&s->rate,nullptr,0)!=MA_SUCCESS ||
  s->channels<1 || s->channels>2 || s->rate<8000 || s->rate>192000 ||
  ma_decoder_get_length_in_pcm_frames(&s->decoder,&s->frames)!=MA_SUCCESS || !s->frames || s->frames>(uint64_t(1)<<48) ||
  ma_decoder_seek_to_pcm_frame(&s->decoder,0)!=MA_SUCCESS)return 0;
 return reinterpret_cast<uintptr_t>(s.release());
}
uint32_t riffine_browser_decoder_channels(BrowserDecoder *s){return s ? s->channels : 0;}
uint32_t riffine_browser_decoder_rate(BrowserDecoder *s){return s ? s->rate : 0;}
uint64_t riffine_browser_decoder_frames(BrowserDecoder *s){return s ? s->frames : 0;}
int riffine_browser_decoder_seek(BrowserDecoder *s,uint64_t frame){
 if(!s || frame>s->frames)return 1;
 // Match native exact MP3 seeking: rewind and decode forward to reconstruct the bit reservoir.
 if(s->codec==1){
  if(ma_decoder_seek_to_pcm_frame(&s->decoder,0)!=MA_SUCCESS)return 1;
  for(uint64_t done=0;done<frame;){
   auto count=uint32_t(std::min<uint64_t>(8192,frame-done));ma_uint64 n=0;
   auto result=ma_decoder_read_pcm_frames(&s->decoder,s->scratch.data(),count,&n);
   if((result!=MA_SUCCESS && result!=MA_AT_END)||n!=count)return 1;done+=n;
  }
  return 0;
 }
 return ma_decoder_seek_to_pcm_frame(&s->decoder,frame)!=MA_SUCCESS;
}
int riffine_browser_decoder_read(BrowserDecoder *s,float *pcm,uint32_t count){
 if(!s || !pcm || !count || count>8192)return -1;
 ma_uint64 n=0;auto result=ma_decoder_read_pcm_frames(&s->decoder,pcm,count,&n);
 if((result!=MA_SUCCESS && result!=MA_AT_END)||n>count)return -1;
 for(size_t i=0;i<size_t(n)*s->channels;++i)if(!std::isfinite(pcm[i]))return -1;
 return int(n);
}
void riffine_browser_decoder_destroy(BrowserDecoder *s){delete s;}
}
