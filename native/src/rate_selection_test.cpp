#include "riffine_rate_selection.h"
#include "riffine_selection.h"
#include <algorithm>
#include <array>
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <new>
#include <memory>
#include <stdexcept>
#include <vector>
static bool track=false;static unsigned allocations=0;
void *operator new(size_t n){if(track)++allocations;if(auto p=std::malloc(n?n:1))return p;throw std::bad_alloc();}
void *operator new[](size_t n){return ::operator new(n);}
void operator delete(void *p) noexcept{std::free(p);}
void operator delete[](void *p) noexcept{std::free(p);}
void operator delete(void *p,size_t) noexcept{std::free(p);}
void operator delete[](void *p,size_t) noexcept{std::free(p);}
namespace {
char error[512];void check(bool b,const char *message){if(!b)throw std::runtime_error(message);}void ok(int r){if(r)throw std::runtime_error(error);}
void run(unsigned sourceRate,unsigned outputRate,double speed,double pitch){
 riffine_rate_selection *raw=nullptr;ok(riffine_rate_selection_create(2,sourceRate,outputRate,sourceRate,speed,pitch,&raw,error,sizeof(error)));
 std::unique_ptr<riffine_rate_selection,decltype(&riffine_rate_selection_destroy)> owner(raw,riffine_rate_selection_destroy);
 riffine_rate_selection_info info{};ok(riffine_rate_selection_info_get(raw,&info,error,sizeof(error)));
 check(info.source_rate==sourceRate&&info.output_rate==outputRate&&info.output_frames==uint64_t(std::llround(double(sourceRate)*outputRate/sourceRate/speed))&&!info.consumed_source_frames,"rate info mismatch");
 std::array<float,1024> input{};std::array<float,226> output{};uint64_t read=0,written=0,measured=0,crossings[2]{};float previous[2]{};double energy[2]{};
 allocations=0;track=true;
 for(;;){uint32_t count=0,need=0;ok(riffine_rate_selection_read(raw,output.data(),113,&count,&need,error,sizeof(error)));
  for(unsigned i=0;i<count;++i){bool measure=written+i>outputRate/10&&written+i+outputRate/10<info.output_frames;
   for(unsigned c=0;c<2;++c){auto value=output[i*2+c];check(std::isfinite(value),"nonfinite resampled output");
    if(measure){if(previous[c]<=0&&value>0)++crossings[c];energy[c]+=double(value)*value;}previous[c]=value;
    if(written+i==0||written+i+1==info.output_frames)check(value==0,"device edge fade missing");
   }if(measure)++measured;
  }written+=count;
  if(need){auto frames=uint32_t(std::min<uint64_t>(512,sourceRate-read));check(frames>0,"requested source beyond bounds");
   for(unsigned i=0;i<frames;++i)for(unsigned c=0;c<2;++c)input[i*2+c]=float(.2*std::sin(6.283185307179586*(c?660:440)*double(read+i)/sourceRate));
   ok(riffine_rate_selection_push(raw,input.data(),frames,error,sizeof(error)));read+=frames;
  }else if(!count)break;
 }
 track=false;check(!allocations&&read==sourceRate&&written==info.output_frames&&measured,"rate storage/duration mismatch");
 ok(riffine_rate_selection_info_get(raw,&info,error,sizeof(error)));check(info.consumed_source_frames==sourceRate,"rate source consumption mismatch");
 double hz[2]{},rms[2]{};for(unsigned c=0;c<2;++c){hz[c]=crossings[c]*double(outputRate)/measured;rms[c]=std::sqrt(energy[c]/measured);auto expected=(c?660:440)*std::pow(2,pitch/12);check(std::abs(hz[c]-expected)<4&&rms[c]>.02&&rms[c]<.3,"device-rate signal mismatch");}
 std::printf(R"({"sourceRate":%u,"outputRate":%u,"speed":%.2f,"pitch":%.0f,"frames":%llu,"latencyFrames":%u,"frequency":[%.6f,%.6f],"rms":[%.9f,%.9f]})" "\n",sourceRate,outputRate,speed,pitch,(unsigned long long)written,info.resampler_latency_frames,hz[0],hz[1],rms[0],rms[1]);
}
void alias(){
 riffine_rate_selection *raw=nullptr;ok(riffine_rate_selection_create(1,48000,8000,48000,1,0,&raw,error,sizeof(error)));
 std::unique_ptr<riffine_rate_selection,decltype(&riffine_rate_selection_destroy)> owner(raw,riffine_rate_selection_destroy);
 std::array<float,512> input{},output{};uint64_t read=0,written=0,measured=0;double energy=0;
 for(;;){uint32_t count=0,need=0;ok(riffine_rate_selection_read(raw,output.data(),512,&count,&need,error,sizeof(error)));
  for(unsigned i=0;i<count;++i)if(written+i>800&&written+i<7200){energy+=double(output[i])*output[i];++measured;}written+=count;
  if(need){auto frames=uint32_t(std::min<uint64_t>(512,48000-read));for(unsigned i=0;i<frames;++i)input[i]=float(.2*std::sin(6.283185307179586*10000*double(read+i)/48000));ok(riffine_rate_selection_push(raw,input.data(),frames,error,sizeof(error)));read+=frames;}else if(!count)break;
 }
 check(written==8000&&measured&&std::sqrt(energy/measured)<.01,"downsample alias attenuation failed");
}
void tiny(){for(auto rates:{std::pair<unsigned,unsigned>{8000,192000},{192000,8000},{48000,44100}}){
 riffine_rate_selection *raw=nullptr;ok(riffine_rate_selection_create(1,rates.first,rates.second,1,2,0,&raw,error,sizeof(error)));
 std::unique_ptr<riffine_rate_selection,decltype(&riffine_rate_selection_destroy)> owner(raw,riffine_rate_selection_destroy);
 uint64_t frames=0;float input=.2f,output=0;bool fed=false;
 for(;;){uint32_t n=0,need=0;ok(riffine_rate_selection_read(raw,&output,1,&n,&need,error,sizeof(error)));frames+=n;
  if(need){check(!fed,"tiny requested repeat input");ok(riffine_rate_selection_push(raw,&input,1,error,sizeof(error)));fed=true;}else if(!n)break;}
 check(fed&&frames==std::max<uint64_t>(1,uint64_t(std::llround(double(rates.second)/rates.first/2))),"tiny device count differs");
 }}
void equal(){
 riffine_selection *base=nullptr;riffine_rate_selection *rate=nullptr;
 ok(riffine_selection_create(1,48000,4800,.75,3,&base,error,sizeof(error)));ok(riffine_rate_selection_create(1,48000,48000,4800,.75,3,&rate,error,sizeof(error)));
 std::unique_ptr<riffine_selection,decltype(&riffine_selection_destroy)> b(base,riffine_selection_destroy);
 std::unique_ptr<riffine_rate_selection,decltype(&riffine_rate_selection_destroy)> r(rate,riffine_rate_selection_destroy);
 auto collect=[&](bool convert){std::vector<float> result;std::array<float,512> input{},output{};uint64_t read=0;
  for(;;){uint32_t n=0,need=0;if(convert)ok(riffine_rate_selection_read(rate,output.data(),512,&n,&need,error,sizeof(error)));else ok(riffine_selection_read(base,output.data(),512,&n,&need,error,sizeof(error)));
   result.insert(result.end(),output.begin(),output.begin()+n);
   if(need){auto count=uint32_t(std::min<uint64_t>(512,4800-read));for(unsigned i=0;i<count;++i)input[i]=float(.2*std::sin(6.283185307179586*440*double(read+i)/48000));
    if(convert)ok(riffine_rate_selection_push(rate,input.data(),count,error,sizeof(error)));else ok(riffine_selection_push(base,input.data(),count,error,sizeof(error)));read+=count;
   }else if(!n)break;
  }return result;};check(collect(false)==collect(true),"equal-rate wrapper changed shared PCM");
}
}
int main(){try{tiny();equal();alias();for(auto rates:{std::pair<unsigned,unsigned>{48000,44100},{44100,48000},{8000,192000},{192000,8000},{48000,48000}})for(auto profile:{std::pair<double,double>{.25,-12},{1,3},{2,12}})run(rates.first,rates.second,profile.first,profile.second);return 0;}catch(const std::exception &e){track=false;std::fprintf(stderr,"%s\n",e.what());return 1;}}
