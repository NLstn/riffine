#include "riffine_selection.h"
#include <algorithm>
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <new>
#include <stdexcept>
#include <vector>

static bool trackAllocations=false;
static unsigned allocations=0;
void *operator new(size_t size) {
 if(trackAllocations) ++allocations;
 if(void *p=std::malloc(size ? size : 1)) return p;
 throw std::bad_alloc();
}
void *operator new[](size_t size) {return ::operator new(size);}
void operator delete(void *p) noexcept {std::free(p);}
void operator delete[](void *p) noexcept {std::free(p);}
void operator delete(void *p,size_t) noexcept {std::free(p);}
void operator delete[](void *p,size_t) noexcept {std::free(p);}
namespace {
char error[512];
void check(bool ok,const char *message) {if(!ok) throw std::runtime_error(message);}
void ok(int status) {if(status) throw std::runtime_error(error);}
struct Result {double speed,pitch,frequency[2],rms[2];uint64_t frames;};
Result run(double speed,double pitch) {
 riffine_selection *selection=nullptr;
 ok(riffine_selection_create(2,48000,48000,speed,pitch,&selection,error,sizeof(error)));
 try {
  riffine_selection_info info{};ok(riffine_selection_info_get(selection,&info,error,sizeof(error)));
  check(info.output_frames==uint64_t(std::llround(48000/speed)) && info.consumed_source_frames==0,"bad initial info");
  float input[1024]{},output[226]{};uint32_t count=1,needsInput=1;
  check(riffine_selection_read(selection,output,0,&count,&needsInput,error,sizeof(error)) && !count,"invalid read accepted");
  check(riffine_selection_push(selection,input,513,error,sizeof(error)),"oversized input accepted");
  uint64_t read=0,written=0,measured=0,crossings[2]{};double energy[2]{};float previous[2]{};
  allocations=0;trackAllocations=true;
  for(;;) {
   ok(riffine_selection_read(selection,output,113,&count,&needsInput,error,sizeof(error)));
   for(uint32_t i=0;i<count;++i) {
    for(unsigned c=0;c<2;++c) {
     float value=output[i*2+c];check(std::isfinite(value),"nonfinite selection");
     if(written+i>4800 && written+i+4800<info.output_frames) {
      if(previous[c]<=0 && value>0) ++crossings[c];energy[c]+=double(value)*value;
     }
     if(written+i==0 || written+i==info.output_frames-1) check(value==0,"edge fade missing");
     previous[c]=value;
    }
    if(written+i>4800 && written+i+4800<info.output_frames) ++measured;
   }
   written+=count;
   if(needsInput) {
    auto frames=uint32_t(std::min<uint64_t>(512,48000-read));
    for(uint32_t i=0;i<frames;++i) for(unsigned c=0;c<2;++c) input[i*2+c]=float(.2*std::sin(2*3.141592653589793*(c ? 660 : 440)*double(read+i)/48000));
    ok(riffine_selection_push(selection,input,frames,error,sizeof(error)));read+=frames;
   } else if(!count) break;
  }
  trackAllocations=false;check(!allocations,"selection allocated after creation");
  check(written==info.output_frames && read==48000 && measured>0,"wrong selection count");
  ok(riffine_selection_info_get(selection,&info,error,sizeof(error)));check(info.consumed_source_frames==read,"wrong source count");
  check(riffine_selection_push(selection,input,1,error,sizeof(error)),"input accepted after EOF");
  ok(riffine_selection_read(selection,output,113,&count,&needsInput,error,sizeof(error)));check(!count && !needsInput,"EOF not idempotent");
  Result result{speed,pitch,{0,0},{0,0},written};
  for(unsigned c=0;c<2;++c) {
   result.frequency[c]=crossings[c]*48000./measured;result.rms[c]=std::sqrt(energy[c]/measured);
   double hz=(c ? 660 : 440)*std::pow(2,pitch/12);
   check(std::abs(result.frequency[c]-hz)<4 && result.rms[c]>.02 && result.rms[c]<.3,"selection signal mismatch");
  }
  riffine_selection_destroy(selection);return result;
 } catch(...) {trackAllocations=false;riffine_selection_destroy(selection);throw;}
}
std::vector<float> faded(uint64_t frames,unsigned rate,int64_t fade) {
 riffine_selection *selection=nullptr;
 ok(riffine_selection_create(1,rate,frames,1,0,&selection,error,sizeof(error)));
 try {
  if(fade>=0) {
   check(riffine_selection_set_edge_fade_frames(selection,(frames+1)/2+1,error,sizeof(error)),"oversized fade accepted");
   ok(riffine_selection_set_edge_fade_frames(selection,uint64_t(fade),error,sizeof(error)));
  }
  std::vector<float> result(size_t(frames),0);float input[512]{},output[113]{};
  uint64_t read=0,written=0;uint32_t count=0,needsInput=0;
  allocations=0;trackAllocations=true;
  for(;;) {
   ok(riffine_selection_read(selection,output,113,&count,&needsInput,error,sizeof(error)));
   check(written+count<=frames,"fade changed output duration");
   std::copy(output,output+count,result.data()+written);written+=count;
   if(needsInput) {
    auto block=uint32_t(std::min<uint64_t>(512,frames-read));
    for(uint32_t i=0;i<block;++i) input[i]=float(.2*std::cos(2*3.141592653589793*440*double(read+i)/rate));
    ok(riffine_selection_push(selection,input,block,error,sizeof(error)));read+=block;
    // Error path may allocate an exception; separately verify its nonmutation.
    trackAllocations=false;
    check(riffine_selection_set_edge_fade_frames(selection,0,error,sizeof(error)),"fade changed after processing");
    trackAllocations=true;
   } else if(!count) break;
  }
  trackAllocations=false;check(!allocations,"scheduled fade processing allocated");
  check(written==frames && read==frames,"scheduled fade changed counts");
  riffine_selection_destroy(selection);return result;
 } catch(...) {trackAllocations=false;riffine_selection_destroy(selection);throw;}
}
void scheduledFades() {
 for(unsigned rate: {8000u,44100u,48000u,192000u}) for(uint64_t frames: {uint64_t(1),uint64_t(2),uint64_t(3),uint64_t(4095)}) {
  auto baseline=faded(frames,rate,0);
  auto fade=std::min<uint64_t>(rate/200,(frames+1)/2);
  auto explicitFade=faded(frames,rate,int64_t(fade));
  auto defaults=faded(frames,rate,-1);
  auto defaultFade=std::min<uint64_t>(uint64_t(std::llround(rate*.005)),frames/2);
  for(uint64_t i=0;i<frames;++i) {
   auto gain=[&](uint64_t length) {return length ? std::min({1.,double(i)/length,double(frames-1-i)/length}) : 1.;};
   check(std::abs(explicitFade[size_t(i)]-baseline[size_t(i)]*gain(fade))<2e-7,"scheduled fade gain mismatch");
   check(std::abs(defaults[size_t(i)]-baseline[size_t(i)]*gain(defaultFade))<2e-7,"default preview fade changed");
  }
 }
 check(riffine_selection_set_edge_fade_frames(nullptr,0,error,sizeof(error)),"null fade selection accepted");
}
void edgeCases() {
 riffine_selection *selection=nullptr;
 check(riffine_selection_create(2,48000,0,1,0,&selection,error,sizeof(error)) && !selection,"empty selection accepted");
 check(riffine_selection_create(2,48000,(uint64_t(1)<<48)+1,1,0,&selection,error,sizeof(error)) && !selection,"huge selection accepted");
 for(unsigned rate: {8000u,48000u,192000u}) {
  ok(riffine_selection_create(1,rate,1,2,0,&selection,error,sizeof(error)));
  float input=INFINITY,output[1]{};uint32_t count=0,needsInput=0;
  check(riffine_selection_push(selection,&input,1,error,sizeof(error)),"nonfinite input accepted");
  riffine_selection_info info{};ok(riffine_selection_info_get(selection,&info,error,sizeof(error)));check(!info.consumed_source_frames,"rejected input advanced source");
  input=.2f;ok(riffine_selection_push(selection,&input,1,error,sizeof(error)));
  ok(riffine_selection_read(selection,output,1,&count,&needsInput,error,sizeof(error)));check(count==1 && !needsInput,"single frame round duration changed");
  ok(riffine_selection_read(selection,output,1,&count,&needsInput,error,sizeof(error)));check(!count && !needsInput,"single frame EOF changed");
  riffine_selection_destroy(selection);
 }
}
}
int main() {
 try {
  edgeCases();scheduledFades();const double speeds[]={.25,.5,.75,1.3,2},pitches[]={0,-12,3,7,12};Result results[5]{};
  for(unsigned i=0;i<5;++i) results[i]=run(speeds[i],pitches[i]);
  std::printf("{\"version\":1,\"sampleRate\":48000,\"channels\":2,\"sourceFrames\":48000,\"cases\":[");
  for(unsigned i=0;i<5;++i) {
   const auto &r=results[i];if(i) std::putchar(',');
   std::printf("{\"speed\":%.6f,\"pitch\":%.6f,\"outputFrames\":%llu,\"frequency\":[%.6f,%.6f],\"rms\":[%.6f,%.6f]}",r.speed,r.pitch,(unsigned long long)r.frames,r.frequency[0],r.frequency[1],r.rms[0],r.rms[1]);
  }
  std::puts("]}");return 0;
 } catch(const std::exception &e) {std::fprintf(stderr,"selection test: %s\n",e.what());return 1;}
}
