#include "riffine_decode.h"
#define NOMINMAX
#include "miniaudio.h"
#include "encoded_file.h"
#include <algorithm>
#include <array>
#include <cmath>
#include <cstdio>
#include <filesystem>
#include <memory>
#include <stdexcept>
#include <string>
#include "windows_m4a.h"

struct riffine_decoder {
 std::shared_ptr<riffine::EncodedFile> resource;
#ifdef _WIN32
 std::unique_ptr<riffine::WindowsM4A> m4a;
#endif
 ma_decoder decoder{};
 riffine_decoder_info info{};
 bool opened=false,failed=false;
 uint64_t position=0;
 std::array<float,8192*2> scratch{};
 ~riffine_decoder(){if(opened)ma_decoder_uninit(&decoder);}
};
namespace {
void require(bool ok,const char *text){if(!ok)throw std::runtime_error(text);}
template<class F>int boundary(char *error,size_t size,F f) noexcept{
 if(error&&size)error[0]=0;
 try{f();return 0;}catch(const std::exception &e){if(error&&size)std::snprintf(error,size,"%s",e.what());}
 catch(...){if(error&&size)std::snprintf(error,size,"unexpected decoder error");}return 1;
}
struct File {
 FILE *value=nullptr;
 explicit File(const char *path){
#ifdef _WIN32
 value=_wfopen(std::filesystem::u8path(path).c_str(),L"rb");
#else
 value=std::fopen(path,"rb");
#endif
 require(value!=nullptr,"cannot open original audio");
 }
 ~File(){std::fclose(value);}
 void seek(uint64_t pos){
#ifdef _WIN32
 require(_fseeki64(value,ma_int64(pos),SEEK_SET)==0,"cannot seek audio header");
#else
 require(fseeko(value,off_t(pos),SEEK_SET)==0,"cannot seek audio header");
#endif
 }
 void read(void *data,size_t n){require(std::fread(data,1,n,value)==n,"truncated audio header");}
};
unsigned u16(const unsigned char *b){return unsigned(b[0])|(unsigned(b[1])<<8);}
uint32_t u32(const unsigned char *b){return uint32_t(b[0])|(uint32_t(b[1])<<8)|(uint32_t(b[2])<<16)|(uint32_t(b[3])<<24);}
template<class Reader>uint32_t precision(Reader &file,uint32_t encoding){
 if(encoding==3)return 0;
 unsigned char h[26]{};
 if(encoding==2){file.read(h,sizeof(h));if(std::string(reinterpret_cast<char*>(h),4)!="fLaC"|| (h[4]&127)!=0 || ((uint32_t(h[5])<<16)|(uint32_t(h[6])<<8)|h[7])!=34)return 0;uint64_t packed=0;for(unsigned i=18;i<26;++i)packed=(packed<<8)|h[i];return uint32_t(((packed>>36)&31)+1);}
 file.read(h,12);require(std::string(reinterpret_cast<char*>(h),4)=="RIFF"&&std::string(reinterpret_cast<char*>(h+8),4)=="WAVE","unsupported WAV container");
 uint64_t end=uint64_t(u32(h+4))+8,pos=12;
 while(pos<end){require(end-pos>=8,"truncated WAV chunk");file.seek(pos);file.read(h,8);uint32_t n=u32(h+4);require(uint64_t(n)<=end-pos-8,"invalid WAV chunk length");
 if(std::string(reinterpret_cast<char*>(h),4)=="fmt "){require(n>=16,"truncated WAV format");file.read(h,16);require(u16(h)==1,"only integer PCM WAV is supported");auto bits=u16(h+14);require(bits==16||bits==24||bits==32,"unsupported WAV precision");return bits;}
 pos+=8+uint64_t(n)+(n&1);
 }
 throw std::runtime_error("missing WAV format");
}
struct ResourceHeader {
 riffine::EncodedFile &resource;uint64_t position=0;
 void seek(uint64_t pos){position=pos;}
 void read(void *output,size_t count){require(resource.readAt(output,count,position)==count,"truncated audio header");position+=count;}
};
ma_result resourceRead(ma_decoder *decoder,void *output,size_t bytes,size_t *read){
 if(read)*read=0;
 try{auto count=static_cast<riffine::EncodedFile *>(decoder->pUserData)->read(output,bytes);if(read)*read=count;return count||!bytes?MA_SUCCESS:MA_AT_END;}
 catch(...){return MA_ERROR;}
}
ma_result resourceSeek(ma_decoder *decoder,ma_int64 offset,ma_seek_origin origin){
 return static_cast<riffine::EncodedFile *>(decoder->pUserData)->seek(offset,unsigned(origin))?MA_SUCCESS:MA_BAD_SEEK;
}
void finishOpen(riffine_decoder *s){
#ifdef _WIN32
 if(s->m4a){s->info.channels=s->m4a->channels;s->info.sample_rate=s->m4a->sampleRate;s->info.encoding=s->m4a->encoding;s->info.frames=s->m4a->frames;return;}
#endif
 require(s->opened,"unsupported or malformed audio (tested codecs: WAV PCM, FLAC, MP3)");
 s->info.channels=s->decoder.outputChannels;s->info.sample_rate=s->decoder.outputSampleRate;
 require(s->info.channels>=1&&s->info.channels<=2&&s->info.sample_rate>=8000&&s->info.sample_rate<=192000,"unsupported source rate or channels");
 ma_uint64 length=0;require(ma_decoder_get_length_in_pcm_frames(&s->decoder,&length)==MA_SUCCESS&&length>0&&length<=(uint64_t(1)<<48),"invalid or unknown decoded source length");s->info.frames=length;
}
void readExact(riffine_decoder *s,float *out,uint32_t frames){
 ma_uint64 read=0;ma_result result=MA_SUCCESS;
#ifdef _WIN32
 if(s->m4a)read=s->m4a->read(out,frames);else
#endif
 result=ma_decoder_read_pcm_frames(&s->decoder,out,frames,&read);
 require((result==MA_SUCCESS||result==MA_AT_END)&&read==frames,"truncated or malformed decoded audio");
 require(s->info.decoded_frames<=UINT64_MAX-frames,"decoded diagnostic counter overflow");s->info.decoded_frames+=frames;
 for(size_t i=0;i<size_t(frames)*s->info.channels;++i)require(std::isfinite(out[i]),"non-finite decoded sample");
}
#ifdef _WIN32
void tryM4A(riffine_decoder *s){
 if(s->opened)return;
 unsigned char header[12]{};
 if(s->resource->readAt(header,sizeof(header),0)!=sizeof(header)||std::memcmp(header+4,"ftyp",4)!=0)return;
 s->m4a=std::make_unique<riffine::WindowsM4A>(s->resource);
}
#endif
}
int riffine_decoder_open_file(const char *path,riffine_decoder **decoder,char *error,size_t size){
 if(decoder)*decoder=nullptr;
 return boundary(error,size,[&]{require(path&&decoder,"path and decoder result are required");auto s=std::make_unique<riffine_decoder>();
 const ma_encoding_format formats[]={ma_encoding_format_wav,ma_encoding_format_flac,ma_encoding_format_mp3};
 for(unsigned i=0;i<3;++i){auto config=ma_decoder_config_init(ma_format_f32,0,0);config.encodingFormat=formats[i];
#ifdef _WIN32
 auto result=ma_decoder_init_file_w(std::filesystem::u8path(path).c_str(),&config,&s->decoder);
#else
 auto result=ma_decoder_init_file(path,&config,&s->decoder);
#endif
 if(result==MA_SUCCESS){s->opened=true;s->info.encoding=i+1;break;}}
#ifdef _WIN32
 if(!s->opened){
  auto file=CreateFileW(std::filesystem::u8path(path).c_str(),GENERIC_READ,FILE_SHARE_READ|FILE_SHARE_DELETE,nullptr,OPEN_EXISTING,FILE_ATTRIBUTE_NORMAL,nullptr);
  require(file!=INVALID_HANDLE_VALUE,"cannot open original audio");
  try{s->resource=std::make_shared<riffine::EncodedFile>(uint64_t(uintptr_t(file)));}catch(...){CloseHandle(file);throw;}
  CloseHandle(file);tryM4A(s.get());
 }
#endif
 finishOpen(s.get());
 if(s->info.encoding<3){File header(path);s->info.source_bits=precision(header,s->info.encoding);}
 if(s->opened)require(ma_decoder_seek_to_pcm_frame(&s->decoder,0)==MA_SUCCESS,"cannot reset source decoder");*decoder=s.release();});
}
int riffine_decoder_open_handle(uint64_t handle,riffine_decoder **decoder,char *error,size_t size){
 if(decoder)*decoder=nullptr;
 return boundary(error,size,[&]{
  require(decoder,"decoder result required");auto s=std::make_unique<riffine_decoder>();s->resource=std::make_shared<riffine::EncodedFile>(handle);
  const ma_encoding_format formats[]={ma_encoding_format_wav,ma_encoding_format_flac,ma_encoding_format_mp3};
  for(unsigned i=0;i<3;++i){
   require(s->resource->seek(0,0),"cannot reset resource cursor");
   auto config=ma_decoder_config_init(ma_format_f32,0,0);config.encodingFormat=formats[i];
   auto result=ma_decoder_init(resourceRead,resourceSeek,s->resource.get(),&config,&s->decoder);
   if(result==MA_SUCCESS){s->opened=true;s->info.encoding=i+1;break;}
  }
#ifdef _WIN32
  tryM4A(s.get());
#endif
  finishOpen(s.get());if(s->info.encoding<3){ResourceHeader header{*s->resource};s->info.source_bits=precision(header,s->info.encoding);}
  if(s->opened)require(ma_decoder_seek_to_pcm_frame(&s->decoder,0)==MA_SUCCESS,"cannot reset source decoder");*decoder=s.release();
 });
}
int riffine_decoder_info_get(const riffine_decoder *s,riffine_decoder_info *info,char *error,size_t size){return boundary(error,size,[&]{require(s&&info,"decoder and info required");*info=s->info;});}
int riffine_decoder_seek(riffine_decoder *s,uint64_t frame,char *error,size_t size){return boundary(error,size,[&]{require(s&&!s->failed&&frame<=s->info.frames,"invalid decoder seek");s->failed=true;
#ifdef _WIN32
 if(s->m4a){s->m4a->rewind();for(uint64_t done=0;done<frame;){auto n=uint32_t(std::min<uint64_t>(8192,frame-done));readExact(s,s->scratch.data(),n);done+=n;}}
 else
#endif
 if(s->info.encoding==3){require(ma_decoder_seek_to_pcm_frame(&s->decoder,0)==MA_SUCCESS,"cannot rewind MP3");for(uint64_t done=0;done<frame;){auto n=uint32_t(std::min<uint64_t>(8192,frame-done));readExact(s,s->scratch.data(),n);done+=n;}}
 else require(ma_decoder_seek_to_pcm_frame(&s->decoder,frame)==MA_SUCCESS,"cannot seek decoded source");
 s->position=frame;s->failed=false;});}
int riffine_decoder_read(riffine_decoder *s,float *out,uint32_t capacity,uint32_t *produced,char *error,size_t size){if(produced)*produced=0;return boundary(error,size,[&]{require(s&&out&&produced&&capacity>=1&&capacity<=8192,"invalid decoder read buffer");require(!s->failed,"decoder failed; recreate it");s->failed=true;auto n=uint32_t(std::min<uint64_t>(capacity,s->info.frames-s->position));if(n)readExact(s,out,n);s->position+=n;*produced=n;s->failed=false;});}
void riffine_decoder_close(riffine_decoder *s){delete s;}
