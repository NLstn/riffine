#ifndef RIFFINE_ALAC_SOURCE_H
#define RIFFINE_ALAC_SOURCE_H
#ifdef RIFFINE_ALAC
#include "encoded_file.h"
#include <array>
#include <memory>
extern "C" {
int riffine_alac_open(void *,int64_t (*)(void *,unsigned char *,size_t,uint64_t),uint64_t,void **,uint32_t *,uint32_t *,char *,size_t);
int riffine_alac_read(void *,float *,uint32_t,uint32_t *,char *,size_t);
void riffine_alac_close(void *);
}
namespace riffine {
class AlacSource {
 std::shared_ptr<EncodedFile> resource;
 void *decoder=nullptr;
 static int64_t readAt(void *user,unsigned char *output,size_t size,uint64_t offset) noexcept {
  try{return int64_t(static_cast<EncodedFile *>(user)->readAt(output,size,offset));}catch(...){return -1;}
 }
 void open(){
  riffine_alac_close(decoder);decoder=nullptr;char error[512];uint32_t count=0,rate=0;
  if(riffine_alac_open(resource.get(),readAt,resource->size(),&decoder,&count,&rate,error,sizeof(error)))throw std::runtime_error(error);
  if(channels&&(channels!=count||sampleRate!=rate))throw std::runtime_error("ALAC source format changed");
  channels=count;sampleRate=rate;
 }
public:
 uint32_t channels=0,sampleRate=0;
 uint64_t frames=0;
 explicit AlacSource(std::shared_ptr<EncodedFile> resource):resource(std::move(resource)) {
  try {
   open();std::array<float,8192*2> scratch{};
   for(;;){auto n=read(scratch.data(),8192);if(!n)break;frames+=n;if(frames>(uint64_t(1)<<48))throw std::runtime_error("ALAC source is too long");}
   if(!frames)throw std::runtime_error("ALAC contains no decoded audio");
   open();
  }catch(...){riffine_alac_close(decoder);decoder=nullptr;throw;}
 }
 ~AlacSource(){riffine_alac_close(decoder);}
 uint32_t read(float *output,uint32_t capacity){uint32_t produced=0;char error[512];if(riffine_alac_read(decoder,output,capacity,&produced,error,sizeof(error)))throw std::runtime_error(error);return produced;}
 void rewind(){open();}
};
}
#endif
#endif
