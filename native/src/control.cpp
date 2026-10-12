#include "riffine_control.h"
#include "riffine_scheduled.h"
#include <array>
#include <cstdio>
#include <cstring>
#include <memory>
#include <stdexcept>
#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif
namespace {
constexpr uint64_t magic=0x314350494e464652ULL;
using Words=std::array<uint64_t,16>;
void encode(const Words &words,unsigned char *bytes){for(unsigned i=0;i<16;++i)for(unsigned b=0;b<8;++b)bytes[i*8+b]=static_cast<unsigned char>(words[i]>>(8*b));}
Words decode(const unsigned char *bytes){Words words{};for(unsigned i=0;i<16;++i)for(unsigned b=0;b<8;++b)words[i]|=uint64_t(bytes[i*8+b])<<(8*b);return words;}
double floating(uint64_t word){double value;static_assert(sizeof(value)==sizeof(word));std::memcpy(&value,&word,sizeof(value));return value;}
void reply(riffine_scheduled_worker *worker,uint64_t id,int status,const char *message){
 std::array<unsigned char,640> bytes{};riffine_scheduled_info info{};char error[512];
 if(riffine_scheduled_info_get(worker,&info,error,sizeof(error)))throw std::runtime_error(error);
 uint32_t outputRate=0;if(riffine_scheduled_output_rate_get(worker,&outputRate,error,sizeof(error)))throw std::runtime_error(error);
 Words words{magic,2,id,uint64_t(status),info.channels,info.sample_rate,info.source_frames,info.generation,info.last_sequence,info.pending,outputRate};
 encode(words,bytes.data());if(status)std::snprintf(reinterpret_cast<char*>(bytes.data()+128),512,"%s",message);
 if(std::fwrite(bytes.data(),1,bytes.size(),stdout)!=bytes.size()||std::fflush(stdout)!=0)throw std::runtime_error("cannot write native control response");
}
}
int riffine_scheduled_control(uint64_t handle){
 try {
#ifdef _WIN32
  if(_setmode(_fileno(stdin),_O_BINARY)==-1||_setmode(_fileno(stdout),_O_BINARY)==-1)throw std::runtime_error("cannot set binary control pipes");
#endif
  char error[512];riffine_scheduled_worker *raw=nullptr;
  if(riffine_scheduled_open_handle(handle,&raw,error,sizeof(error)))throw std::runtime_error(error);
  std::unique_ptr<riffine_scheduled_worker,decltype(&riffine_scheduled_close)> owner(raw,riffine_scheduled_close);
  uint64_t last=0;
  for(;;){
   std::array<unsigned char,128> bytes{};size_t n=std::fread(bytes.data(),1,bytes.size(),stdin);
   if(!n&&std::feof(stdin))return 0;
   if(n!=bytes.size())throw std::runtime_error("truncated native control request");
   auto q=decode(bytes.data());
   if(q[0]!=magic||q[1]!=2||last==UINT64_MAX||q[2]!=last+1)throw std::runtime_error("invalid native control envelope");
   last=q[2];int status=0;error[0]=0;
   bool reserved=false;for(unsigned i=13;i<16;++i)reserved|=q[i]!=0;
   if(q[3]!=3)for(unsigned i=5;i<13;++i)reserved|=q[i]!=0;
   if(q[3]!=2&&q[3]!=3&&q[3]!=5)reserved|=q[4]!=0;
   if(reserved){status=1;std::snprintf(error,sizeof(error),"unexpected native control fields");}
   else if(q[3]==1){} // info contains no completion or clock claim
   else if(q[3]==2)status=riffine_scheduled_reset(raw,q[4],error,sizeof(error));
   else if(q[3]==5){if(q[4]>UINT32_MAX){status=1;std::snprintf(error,sizeof(error),"invalid output sample rate");}else status=riffine_scheduled_set_output_rate(raw,uint32_t(q[4]),error,sizeof(error));}
   else if(q[3]==3){
    riffine_schedule schedule{q[4],q[5],q[6],q[7],q[8],q[9],q[10],floating(q[11]),floating(q[12])};
    status=riffine_scheduled_submit(raw,&schedule,error,sizeof(error));
   }else if(q[3]!=4){status=1;std::snprintf(error,sizeof(error),"unknown native control operation");}
   reply(raw,q[2],status,error);
   if(q[3]==4&&!status)return 0;
  }
 }catch(const std::exception &e){std::fprintf(stderr,"%s\n",e.what());return 1;}
 catch(...){std::fprintf(stderr,"unexpected native control error\n");return 1;}
}
