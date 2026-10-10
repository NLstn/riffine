#include "riffine_stream.h"
#include "riffine_pcm_queue.h"
#include <atomic>
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <memory>
#include <new>
#include <stdexcept>
#include <string>
#include <thread>
#include <vector>

static thread_local bool countAllocations=false;
static thread_local uint64_t allocations=0;
void *operator new(size_t size) {
  if (countAllocations) ++allocations;
  if (void *p=std::malloc(size ? size : 1)) return p;
  throw std::bad_alloc();
}
void *operator new[](size_t size) { return ::operator new(size); }
void operator delete(void *p) noexcept { std::free(p); }
void operator delete[](void *p) noexcept { std::free(p); }
void operator delete(void *p, size_t) noexcept { std::free(p); }
void operator delete[](void *p, size_t) noexcept { std::free(p); }
namespace {
void check(bool ok,const char *text) { if (!ok) throw std::runtime_error(text); }
char error[512];
void ok(int result) { if (result) throw std::runtime_error(error); }
using Stream=std::unique_ptr<riffine_stream,decltype(&riffine_stream_destroy)>;
Stream create(unsigned channels=2,double speed=1,double pitch=0,unsigned block=257,unsigned rate=48000) {
  riffine_stream_config config{1,channels,rate,block,speed,pitch};
  riffine_stream *s=nullptr;
  ok(riffine_stream_create(&config,&s,error,sizeof(error)));
  return Stream(s,riffine_stream_destroy);
}
void invalid() {
  riffine_stream_config config{1,2,48000,257,1,0};
  riffine_stream *s=nullptr;
  config.version=2;
  check(riffine_stream_create(&config,&s,error,sizeof(error)) && !s,"accepted bad ABI version");
  config.version=1; config.max_input_frames=8193;
  check(riffine_stream_create(&config,&s,error,sizeof(error)) && !s,"accepted oversized block");
  config.max_input_frames=257; config.channels=3;
  check(riffine_stream_create(&config,&s,error,sizeof(error)) && !s,"accepted channels");
  auto a=create(), b=create();
  std::vector<float> input(514,.1f), out(2060), expected(2060);
  uint32_t consumed=99,produced=99,n=0,m=0;
  check(riffine_stream_process(a.get(),input.data(),257,out.data(),1,&consumed,&produced,error,sizeof(error)) && !consumed && !produced,"capacity failure mutated counts");
  input[5]=NAN;
  check(riffine_stream_process(a.get(),input.data(),257,out.data(),1030,&consumed,&produced,error,sizeof(error)),"accepted NaN");
  input[5]=.1f;
  check(riffine_stream_process(a.get(),input.data(),257,input.data(),257,&consumed,&produced,error,sizeof(error)),"accepted alias");
  check(riffine_stream_configure(a.get(),0,0,error,sizeof(error)),"accepted speed");
  check(riffine_stream_configure(a.get(),1,INFINITY,error,sizeof(error)),"accepted pitch");
  check(riffine_stream_process(a.get(),input.data(),258,out.data(),1030,&consumed,&produced,error,sizeof(error)),"accepted block overflow");
  ok(riffine_stream_process(a.get(),input.data(),257,out.data(),1030,&consumed,&produced,error,sizeof(error)));
  ok(riffine_stream_process(b.get(),input.data(),257,expected.data(),1030,&n,&m,error,sizeof(error)));
  check(n==consumed && m==produced,"invalid call changed planner");
  for (unsigned i=0;i<produced*2;++i) check(out[i]==expected[i],"invalid call changed DSP history");
  auto high=create(2,.25,12,8192,192000);
  riffine_stream_limits limits{}; ok(riffine_stream_get_limits(high.get(),&limits,error,sizeof(error)));
  check(limits.max_input_frames==8192 && limits.max_output_frames==32769 && limits.max_flush_frames>0,"wrong maximum limits");
}
void tone(unsigned channels,double speed,double pitch,unsigned seconds=3,unsigned block=257,bool soak=false) {
  auto stream=create(channels,speed,pitch,block);
  riffine_stream_limits limits{};ok(riffine_stream_get_limits(stream.get(),&limits,error,sizeof(error)));
  auto cap=std::max(limits.max_output_frames,limits.max_flush_frames);
  std::vector<float> in(size_t(block)*channels),out(size_t(cap)*channels);
  uint64_t source=0,output=0,total=uint64_t(seconds)*48000;
  uint64_t crossings[2]{},measured=0;double energy[2]{};float previous[2]{};
  uint64_t skip=uint64_t(std::ceil(limits.input_latency_frames/speed))+limits.output_latency_frames+48000;
  allocations=0;countAllocations=true;
  while (source<total) {
    auto frames=uint32_t(std::min<uint64_t>(block,total-source));
    for (unsigned i=0;i<frames;++i) for(unsigned c=0;c<channels;++c)
      in[size_t(i)*channels+c]=float(.2*std::sin(2*3.141592653589793*(c ? 660 : 440)*double(source+i)/48000));
    uint32_t consumed=0,produced=0;
    ok(riffine_stream_process(stream.get(),in.data(),frames,out.data(),cap,&consumed,&produced,error,sizeof(error)));
    check(consumed==frames && produced<=limits.max_output_frames,"incorrect process counts");
    for(unsigned i=0;i<produced;++i) {
      for(unsigned c=0;c<channels;++c) {
        float sample=out[size_t(i)*channels+c];check(std::isfinite(sample),"nonfinite samples");
        if(output+i>=skip) {
          if(previous[c]<=0 && sample>0) ++crossings[c];
          energy[c]+=double(sample)*sample;
        }
        previous[c]=sample;
      }
      if(output+i>=skip) ++measured;
    }
    source+=consumed;output+=produced;
  }
  countAllocations=false;
  auto processAllocations=allocations;
  check(output==uint64_t(std::floor(total/static_cast<long double>(speed))),"fractional frame drift");
  check(processAllocations==0,"steady-state DSP C++ allocation");
  check(measured>4800,"too little steady-state signal");
  for(unsigned c=0;c<channels;++c) {
    double hz=double(crossings[c])*48000/measured;
    double expected=(c ? 660 : 440)*std::pow(2,pitch/12);
    check(std::abs(hz-expected)<expected*.035,"pitch or channel separation differs");
    check(std::sqrt(energy[c]/measured)>.01,"silent output");
  }
  uint32_t tail=0;
  check(riffine_stream_flush(stream.get(),out.data(),1,&tail,error,sizeof(error)) && tail==0,"accepted short flush capacity");
  ok(riffine_stream_flush(stream.get(),out.data(),cap,&tail,error,sizeof(error)));
  check(tail==uint32_t(std::ceil(limits.input_latency_frames/speed))+limits.output_latency_frames,"incorrect tail count");
  ok(riffine_stream_flush(stream.get(),nullptr,0,&tail,error,sizeof(error)));check(!tail,"duplicate tail");
  uint32_t consumed=0,produced=0;
  check(riffine_stream_process(stream.get(),in.data(),block,out.data(),cap,&consumed,&produced,error,sizeof(error)),"processed after EOF");
  check(riffine_stream_configure(stream.get(),1,0,error,sizeof(error)),"configured after EOF");
  ok(riffine_stream_reset(stream.get(),error,sizeof(error)));
  ok(riffine_stream_flush(stream.get(),nullptr,0,&tail,error,sizeof(error)));check(!tail,"empty reset retained tail");
  if(soak) std::printf("generated-source-seconds=%u source-frames=%llu output-frames=%llu process-cpp-allocations=%llu max-block=%u max-output=%u max-flush=%u\n",seconds,(unsigned long long)source,(unsigned long long)output,(unsigned long long)processAllocations,block,limits.max_output_frames,limits.max_flush_frames);
}
void changes() {
  auto s=create(1,2,0,17);std::vector<float> in(17),out(20000);
  uint32_t consumed=0,produced=0;
  ok(riffine_stream_process(s.get(),in.data(),1,out.data(),20000,&consumed,&produced,error,sizeof(error)));
  check(consumed==1 && produced==0,"subframe carry lost");
  ok(riffine_stream_configure(s.get(),.25,12,error,sizeof(error)));
  ok(riffine_stream_process(s.get(),in.data(),17,out.data(),20000,&consumed,&produced,error,sizeof(error)));
  check(consumed==17 && produced==68,"parameter change counts");
  ok(riffine_stream_configure(s.get(),2,-12,error,sizeof(error)));
  ok(riffine_stream_process(s.get(),in.data(),1,out.data(),20000,&consumed,&produced,error,sizeof(error)));
  check(consumed==1 && produced==1,"configure discarded fractional carry");
  ok(riffine_stream_reset(s.get(),error,sizeof(error)));
  ok(riffine_stream_process(s.get(),in.data(),1,out.data(),20000,&consumed,&produced,error,sizeof(error)));
  check(produced==0,"reset retained fractional carry");
  // Short selections can flush without prerendering or requiring seek-length input.
  uint32_t tail=0;ok(riffine_stream_flush(s.get(),out.data(),20000,&tail,error,sizeof(error)));
  check(tail>0,"short stream has no tail");
}
void liveChanges() {
  auto s=create(1,1,0,257);
  std::vector<float> input(257),output(20000);
  uint64_t source=0;
  for(unsigned segment=0;segment<3;++segment) {
    double speed=segment==0 ? 1 : (segment==1 ? .5 : 2);
    double pitch=segment==0 ? 0 : (segment==1 ? 12 : -12);
    ok(riffine_stream_configure(s.get(),speed,pitch,error,sizeof(error)));
    uint64_t sent=0,received=0,crossings=0,measured=0;float previous=0;
    while(sent<144000) {
      auto frames=uint32_t(std::min<uint64_t>(257,144000-sent));
      for(unsigned i=0;i<frames;++i) input[i]=float(.2*std::sin(2*3.141592653589793*440*double(source+i)/48000));
      uint32_t consumed=0,produced=0;
      ok(riffine_stream_process(s.get(),input.data(),frames,output.data(),20000,&consumed,&produced,error,sizeof(error)));
      for(unsigned i=0;i<produced;++i) {
        if(received+i>=55000) { if(previous<=0 && output[i]>0) ++crossings; ++measured; }
        previous=output[i];
      }
      source+=consumed;sent+=consumed;received+=produced;
    }
    check(received==uint64_t(144000/speed),"live change frame count");
    double hz=double(crossings)*48000/measured,expected=440*std::pow(2,pitch/12);
    check(std::abs(hz-expected)<expected*.035,"live pitch change did not reach output");
  }
}
void resetHistory() {
  auto s=create(2,1,0,257);
  auto fresh=create(2,1,0,257);
  std::vector<float> in(514,.2f),out(2060),expected(2060);
  uint32_t consumed=0,produced=0,n=0,m=0;
  for(int i=0;i<30;++i)
    ok(riffine_stream_process(s.get(),in.data(),257,out.data(),1030,&consumed,&produced,error,sizeof(error)));
  ok(riffine_stream_reset(s.get(),error,sizeof(error)));
  std::fill(in.begin(),in.end(),0.f);
  for(int block=0;block<30;++block) {
    ok(riffine_stream_process(s.get(),in.data(),257,out.data(),1030,&consumed,&produced,error,sizeof(error)));
    ok(riffine_stream_process(fresh.get(),in.data(),257,expected.data(),1030,&n,&m,error,sizeof(error)));
    for(unsigned i=0;i<produced*2;++i) check(std::abs(out[i]-expected[i])<1e-6,"reset leaked old audio");
  }
}
void queue() {
  riffine::PCMQueue q(7,2);float in[16],out[24];
  for(unsigned i=0;i<16;++i) in[i]=float(i+1);
  check(q.write(in,7) && !q.write(in,1),"full queue did not reject write");
  allocations=0;countAllocations=true;
  check(q.read(out,3)==3,"queue partial read");
  check(q.write(in,3),"queue wrap write");
  check(q.read(out,12)==7,"queue underrun count includes silence");
  countAllocations=false;check(!allocations,"callback queue allocated");
  for(unsigned i=0;i<8;++i) check(out[i]==in[i+6],"queue wrap order");
  for(unsigned i=0;i<6;++i) check(out[i+8]==in[i],"queue wrapped suffix");
  for(unsigned i=14;i<24;++i) check(out[i]==0,"underrun did not zero samples");
  q.reset();check(q.read(out,1)==0,"reset retained PCM");
  // Real two-thread SPSC transfer; retain explicit consumed counts on underruns.
  riffine::PCMQueue concurrent(127,2);constexpr uint32_t total=200000;
  std::atomic<bool> failed{false};
  std::thread producer([&] {
    float samples[38];uint32_t sent=0;
    while(sent<total) {
      auto n=std::min(19u,total-sent);
      for(unsigned i=0;i<n;++i) {samples[i*2]=float(sent+i);samples[i*2+1]=-float(sent+i);}
      if(concurrent.write(samples,n)) sent+=n;else std::this_thread::yield();
    }
  });
  std::thread consumer([&] {
    float samples[46];uint32_t received=0;allocations=0;countAllocations=true;
    while(received<total) {
      auto n=concurrent.read(samples,std::min(23u,total-received));
      for(unsigned i=0;i<n;++i)
        if(samples[i*2]!=float(received+i) || samples[i*2+1]!=-float(received+i)) failed=true;
      received+=n;
    }
    countAllocations=false;if(allocations) failed=true;
  });
  producer.join();consumer.join();check(!failed,"concurrent queue order/allocation failure");
}
}
int main(int argc,char **argv) {
  try {
    if(argc==3 && std::string(argv[1])=="--soak") {tone(2,.75,3,unsigned(std::stoul(argv[2])),4096,true);return 0;}
    invalid();changes();liveChanges();resetHistory();queue();
    for(unsigned channels: {1u,2u}) for(double speed: {.25,.5,.75,1.,1.3,2.})
      tone(channels,speed,speed==.25 ? -12 : (speed==2 ? 12 : 0),5);
    std::puts("streaming DSP and bounded PCM queue checks passed");return 0;
  } catch(const std::exception &e) {countAllocations=false;std::fprintf(stderr,"%s\n",e.what());return 1;}
}
