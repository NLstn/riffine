#include "riffine_device_clock.h"
#include "riffine_presentation_queue.h"
#include <array>
#include <atomic>
#include <thread>
#include <cstdlib>
#include <iostream>
#include <new>
#include <stdexcept>
static bool track=false;static unsigned allocations=0;
void *operator new(size_t n){if(track)++allocations;if(auto p=std::malloc(n?n:1))return p;throw std::bad_alloc();}
void *operator new[](size_t n){return ::operator new(n);}
void operator delete(void *p) noexcept{std::free(p);}
void operator delete[](void *p) noexcept{std::free(p);}
void operator delete(void *p,size_t) noexcept{std::free(p);}
void operator delete[](void *p,size_t) noexcept{std::free(p);}
namespace {
void check(bool b,const char *message){if(!b)throw std::runtime_error(message);}
void mapping(){
 riffine::DeviceClockMap clock(100);riffine::PresentationQueue queue(256,1,8);
 std::array<float,256> input{},output{};input.fill(.2f);check(queue.write(input.data(),256)&&queue.start(),"setup");
 uint64_t frame=999,presented=999;
 check(clock.observedFrame(900000,frame)&&frame==0&&!clock.anchored(),"unanchored device advanced output");
 allocations=0;track=true;
 check(clock.callbackFrame(900000,64,frame)&&frame==100&&queue.consume(output.data(),64,frame)==64,"callback timestamp mapping");
 track=false;check(!allocations,"clock/callback mapping allocated");
 check(clock.observedFrame(900099,frame)&&queue.observe(frame,presented)&&presented==0,"device latency acknowledged early");
 check(clock.observedFrame(900132,frame)&&queue.observe(frame,presented)&&presented==32,"latency-compensated partial incorrect");
 check(queue.requestPause(),"pause rejected");
 check(clock.callbackFrame(900064,64,frame)&&!queue.consume(output.data(),64,frame)&&queue.pauseAcknowledged(),"pause callback consumed PCM");
 check(clock.observedFrame(900164,frame)&&queue.observe(frame,presented)&&presented==64,"copied tail not presented");
 // Backend's native counter may restart at zero after fully fenced suspend.
 clock.beginSegment();check(clock.observedFrame(0,frame)&&frame==228,"resumed clock anchor lost continuity");
 check(queue.start()&&clock.callbackFrame(0,64,frame)&&frame==328&&queue.consume(output.data(),64,frame)==64,"resume lost PCM/timestamp continuity");
 check(clock.observedFrame(99,frame)&&queue.observe(frame,presented)&&presented==64,"resume latency acknowledged early");
 check(clock.observedFrame(132,frame)&&queue.observe(frame,presented)&&presented==96,"resumed presentation incorrect");
 check(clock.observedFrame(164,frame)&&queue.observe(frame,presented)&&presented==128,"resumed completion incorrect");
 clock.reset();queue.reset();check(clock.observedFrame(0,frame)&&frame==0&&!clock.anchored(),"reset retained native epoch");
}
void publication(){
 riffine::DeviceClockMap clock(128);std::atomic<int64_t> now{int64_t(1)<<44};std::atomic<bool> done{false};std::atomic<bool> failed{false};
 std::thread writer([&](){for(int64_t i=0;i<20000;++i){uint64_t frame=0;auto sample=(int64_t(1)<<44)+i*64;if(!clock.callbackFrame(sample,64,frame)||frame!=uint64_t(i*64+128)){failed.store(true);break;}now.store(sample,std::memory_order_release);}done.store(true,std::memory_order_release);});
 uint64_t previous=0;while(!done.load(std::memory_order_acquire)){uint64_t frame=0;if(!clock.observedFrame(now.load(std::memory_order_acquire),frame)||frame<previous)failed.store(true);previous=frame;}
 writer.join();check(!failed.load(),"device clock publication raced or regressed");
}
void invalid(){
 uint64_t frame=0;riffine::DeviceClockMap clock(0);
 check(clock.callbackFrame(-1000,64,frame)&&frame==0,"signed device epoch rejected");
 check(clock.observedFrame(-968,frame)&&frame==32,"signed device clock incorrect");
 check(!clock.callbackFrame(-999,64,frame)&&clock.error(),"overlapping timestamp accepted");
 check(!clock.observedFrame(0,frame),"failed clock continued");clock.reset();
 check(!clock.callbackFrame(int64_t(1)<<53,64,frame),"imprecise timestamp accepted");clock.reset();
 check(!clock.callbackFrame(0,8193,frame),"oversized callback accepted");clock.reset();
 check(clock.callbackFrame(0,64,frame),"reset failed");
 check(!clock.callbackFrame(-1,64,frame),"backwards callback clock accepted");
 bool rejected=false;try{riffine::DeviceClockMap bad(192001);}catch(...){rejected=true;}check(rejected,"unbounded latency accepted");
}
}
int main(){try{mapping();invalid();publication();std::cout<<"device latency/epoch/timestamp checks passed\n";return 0;}catch(const std::exception &e){track=false;std::cerr<<e.what()<<"\n";return 1;}}
