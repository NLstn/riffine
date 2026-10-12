#include "riffine_presentation_queue.h"
#include "riffine_schedule_ledger.h"
#include <array>
#include <atomic>
#include <chrono>
#include <cstdlib>
#include <filesystem>
#include <string>
#include <iostream>
#include <new>
#include <stdexcept>
#include <thread>
static thread_local bool track=false;
static thread_local unsigned allocations=0;
void *operator new(size_t n){if(track)++allocations;if(void *p=std::malloc(n?n:1))return p;throw std::bad_alloc();}
void *operator new[](size_t n){return ::operator new(n);}
void operator delete(void *p) noexcept{std::free(p);}
void operator delete[](void *p) noexcept{std::free(p);}
void operator delete(void *p,size_t) noexcept{std::free(p);}
void operator delete[](void *p,size_t) noexcept{std::free(p);}
namespace {
void check(bool ok,const char *message){if(!ok)throw std::runtime_error(message);}
uint32_t consume(riffine::PresentationQueue &queue,float *output,uint32_t frames,uint64_t start){
 allocations=0;track=true;auto n=queue.consume(output,frames,start);track=false;check(!allocations,"callback allocated");return n;
}
void boundaries(){
 riffine::PresentationQueue queue(256,2,4);
 std::array<float,256> input{},output{};for(unsigned i=0;i<256;++i)input[i]=float(i+1);
 check(queue.write(input.data(),128)&&queue.gated(),"initial write/gate");
 check(!consume(queue,output.data(),128,0),"primed copied audio");check(output[0]==0,"primed output not silence");
 check(queue.start(),"start rejected");check(!queue.start(),"duplicate start accepted");
 check(consume(queue,output.data(),64,128)==64,"first PCM count");check(output[0]==1&&output[127]==128,"first PCM bytes");
 uint64_t presented=999;check(queue.observe(128,presented)&&presented==0,"queued frames acknowledged");
 check(queue.observe(160,presented)&&presented==32,"partial presentation incorrect");
 check(queue.observe(159,presented)&&presented==32,"backwards timestamp advanced/reversed progress");
 check(queue.requestPause(),"pause rejected");check(!queue.pauseAcknowledged()&&!queue.gated(),"pause acknowledged before callback boundary");
 check(!consume(queue,output.data(),64,192)&&queue.pauseAcknowledged()&&queue.copied()==64,"pause consumed retained PCM");
 check(queue.observe(256,presented)&&presented==64,"paused copied tail did not finish presenting");
 check(queue.start(),"resume rejected");check(consume(queue,output.data(),64,512)==64&&output[0]==129&&output[127]==256,"resume lost retained PCM");
 check(queue.observe(511,presented)&&presented==64,"paused gap advanced PCM");
 check(queue.observe(544,presented)&&presented==96,"resumed partial mapping wrong");
 check(queue.observe(576,presented)&&presented==128,"resumed tail mapping wrong");
 check(!consume(queue,output.data(),64,576)&&queue.underruns()==64,"underrun not recorded");
 check(queue.observe(640,presented)&&presented==128,"underrun silence counted");
 check(queue.write(input.data(),64),"post-underrun write");check(consume(queue,output.data(),64,640)==64,"post-underrun copy");
 check(queue.observe(672,presented)&&presented==160,"post-underrun prefix wrong");
 check(queue.observe(704,presented)&&presented==192,"post-underrun completion wrong");
 queue.reset();check(!queue.copied()&&!queue.underruns(),"reset retained counters");check(queue.observe(0,presented)&&presented==0,"reset retained timeline");
}
void failClosed(){
 riffine::PresentationQueue queue(256,1,2);std::array<float,256> input{},output{};input.fill(.25f);
 check(queue.write(input.data(),256)&&queue.start(),"overflow setup");
 check(consume(queue,output.data(),64,0)==64&&consume(queue,output.data(),64,64)==64,"timeline setup");
 check(!consume(queue,output.data(),64,128)&&queue.error()&&queue.copied()==128&&queue.timelineError()==3,"overflow copied unmapped PCM");
 for(unsigned i=0;i<64;++i)check(output[i]==0,"overflow emitted stale PCM");
 check(!queue.start()&&!queue.write(input.data(),1)&&!queue.gated(),"failed queue accepted work");
 uint64_t ignored=999;check(!queue.observe(1024,ignored)&&ignored==0,"failed queue advanced presentation without a fence");
 queue.reset();check(queue.write(input.data(),128)&&queue.start(),"overlap setup");check(consume(queue,output.data(),64,0)==64,"overlap prefix");
 check(!consume(queue,output.data(),64,32)&&queue.error()&&queue.copied()==64,"overlapping callback copied PCM");
 queue.reset();input[1]=std::numeric_limits<float>::quiet_NaN();check(!queue.write(input.data(),64),"nonfinite input accepted");input[1]=.25f;
 check(queue.write(input.data(),64)&&queue.start(),"finite write after rejection");
 check(consume(queue,output.data(),64,0)==64,"invalid write changed queue");
 uint64_t presented=0;check(!queue.observe(UINT64_MAX,presented)&&queue.error(),"invalid device position accepted");
}
void scheduleIdentity(){
 riffine::ScheduleLedger ledger;
 riffine_schedule first{1,UINT64_MAX-3,0,128,0,128,0,1,0},second{1,UINT64_MAX-2,0,128,128,256,0,1,0};
 check(!ledger.submit(first),"ledger accepted before generation");check(ledger.reset(1)&&ledger.submit(first)&&ledger.submit(second),"ledger setup failed");
 auto third=second;third.sequence=UINT64_MAX-1;third.output_start=256;third.output_end=384;
 check(!ledger.submit(third)&&ledger.pending()==2,"ledger accepted third schedule");
 std::array<riffine::PresentationAcknowledgement,2> ack{};uint32_t produced=99;
 check(ledger.acknowledge(0,ack.data(),2,produced)&&!produced,"ledger acknowledged queued audio");
 check(ledger.acknowledge(64,ack.data(),2,produced)&&produced==1&&ack[0].sequence==first.sequence&&ack[0].consumed_output_frame==64,"ledger partial identity wrong");
 check(ledger.acknowledge(63,ack.data(),2,produced)&&!produced,"stale ledger position acknowledged");
 check(ledger.acknowledge(64,ack.data(),2,produced)&&!produced,"duplicate ledger acknowledgement");
 check(ledger.acknowledge(256,ack.data(),1,produced)&&produced==1&&ack[0].consumed_output_frame==128&&ledger.pending()==1,"bounded drain skipped first completion");
 check(ledger.acknowledge(256,ack.data(),1,produced)&&produced==1&&ack[0].sequence==second.sequence&&ack[0].consumed_output_frame==256&&!ledger.pending(),"bounded drain lost second completion");
 check(ledger.submit(third),"completed ranges did not release capacity");
 check(!ledger.acknowledge(385,ack.data(),2,produced)&&!produced&&ledger.error(),"presentation beyond schedules accepted");
 check(ledger.reset(2),"ledger recovery reset failed");check(!ledger.submit(first),"stale generation accepted after reset");
 third.generation=2;third.sequence=UINT64_MAX;third.output_start=0;third.output_end=128;
 check(ledger.submit(third),"uint64 maximum schedule ID lost");
 check(ledger.acknowledge(128,ack.data(),2,produced)&&produced==1&&ack[0].generation==2&&ack[0].sequence==UINT64_MAX,"uint64 maximum acknowledgement ID changed");
}
void scheduledPipeline(){
 auto path=std::filesystem::path(RIFFINE_DECODE_FIXTURES)/"generated-stereo.flac";
 char error[512];riffine_scheduled_worker *raw=nullptr;auto name=path.u8string();
 check(!riffine_scheduled_open(name.c_str(),&raw,error,sizeof(error)),error);
 struct Owner{riffine_scheduled_worker *worker;~Owner(){riffine_scheduled_close(worker);}} owner{raw};
 check(!riffine_scheduled_reset(raw,1,error,sizeof(error)),error);
 riffine_schedule first{1,1,12000,36000,0,32000,240,.75,3},second{1,2,4800,9600,32000,34400,240,2,-12};
 riffine::ScheduleLedger ledger;check(ledger.reset(1)&&ledger.submit(first)&&ledger.submit(second),"pipeline ledger setup");
 check(!riffine_scheduled_submit(raw,&first,error,sizeof(error))&&!riffine_scheduled_submit(raw,&second,error,sizeof(error)),error);
 riffine::PresentationQueue queue(4096,2,1024);
 std::array<float,2048> pcm{};std::array<float,256> output{};
 riffine_scheduled_block block{};std::array<riffine::PresentationAcknowledgement,2> acknowledgements{};
 uint64_t queued=0,frame=0,presented=0;uint32_t final_count=0;
 bool started=false,generated_ahead=false;
 for(unsigned tick=0;tick<1000&&presented<34400;++tick){
  for(;;){
   if(!block.frames)check(!riffine_scheduled_read(raw,pcm.data(),1024,&block,error,sizeof(error)),error);
   if(!block.frames)break;
   check(block.output_start==queued,"pipeline producer coordinates lost");
   if(!queue.write(pcm.data(),block.frames))break;
   queued+=block.frames;
   if(block.generated_complete&&presented<block.output_start+block.frames)generated_ahead=true;
   block.frames=0;
  }
  if(!started){check(queued>0&&queue.start(),"pipeline start without prefill");started=true;}
  consume(queue,output.data(),128,frame);frame+=128;
  check(queue.observe(frame>512?frame-512:0,presented),"pipeline presentation failed");
  uint32_t produced=0;check(ledger.acknowledge(presented,acknowledgements.data(),2,produced),"pipeline ledger acknowledgement failed");
  for(unsigned i=0;i<produced;++i){
   const auto &ack=acknowledgements[i];auto end=ack.sequence==1?32000:34400;
   check(ack.generation==1&&ack.consumed_output_frame<=presented,"pipeline acknowledged unpresented PCM");
   if(ack.consumed_output_frame==uint64_t(end)){++final_count;check(ack.sequence==final_count,"pipeline completed schedules out of order");}
  }
 }
 check(generated_ahead&&final_count==2&&presented==34400&&!ledger.pending()&&queue.copied()==34400,"pipeline final counts changed or generation was mistaken for presentation");
}
void threaded(){
 riffine::PresentationQueue queue(32768,2,1024);
 constexpr uint64_t total=786432;
 std::atomic<uint64_t> device{0},observed{0};std::atomic<bool> done{false},failed{false},armed{false};
 auto deadline=std::chrono::steady_clock::now()+std::chrono::seconds(20);
 auto sample=[](uint64_t i,unsigned c){return float((i*2+c)%1009)/1009.f;};
 std::thread producer([&]{
  std::array<float,8192*2> block{};uint64_t offset=0;
  while(offset<total&&!failed.load()){
   auto count=uint32_t(std::min<uint64_t>(8192,total-offset));
   for(unsigned i=0;i<count;++i)for(unsigned c=0;c<2;++c)block[size_t(i)*2+c]=sample(offset+i,c);
   if(queue.write(block.data(),count)){offset+=count;if(offset>=32768&&!armed.exchange(true))queue.start();}
   else std::this_thread::yield();
   if(std::chrono::steady_clock::now()>deadline){failed=true;break;}
  }
 });
 std::thread callback([&]{
  std::array<float,128*2> block{};uint64_t offset=0,frame=0;
  while(!armed.load()&&!failed.load())std::this_thread::yield();
  while(offset<total&&!failed.load()){
   if(offset-observed.load(std::memory_order_acquire)>=32768){std::this_thread::yield();continue;}
   auto count=consume(queue,block.data(),128,frame);
   for(unsigned i=0;i<count;++i)for(unsigned c=0;c<2;++c)if(block[size_t(i)*2+c]!=sample(offset+i,c))failed=true;
   for(unsigned i=count;i<128;++i)for(unsigned c=0;c<2;++c)if(block[size_t(i)*2+c]!=0)failed=true;
   offset+=count;frame+=128;device.store(frame,std::memory_order_release);
   if(queue.error()||std::chrono::steady_clock::now()>deadline)failed=true;
   // This is an accelerated synthetic callback, not a device deadline test.
   std::this_thread::yield();
  }
  done.store(true,std::memory_order_release);
 });
 uint64_t previous=0,presented=0;
 while(!done.load(std::memory_order_acquire)&&!failed.load()){
  if(!queue.observe(device.load(std::memory_order_acquire),presented)||presented<previous||presented>queue.copied())failed=true;
  previous=presented;observed.store(presented,std::memory_order_release);std::this_thread::yield();
 }
 producer.join();callback.join();
 check(!failed.load(),"threaded queue order/timeline failed");
 check(queue.observe(device.load(),presented)&&presented==total&&queue.copied()==total,"threaded presentation did not reach exact final PCM");
}
}
int main(){try{boundaries();failClosed();scheduleIdentity();scheduledPipeline();threaded();std::cout<<"native presentation queue: pause/gaps/overflow/stale clocks, 786432 synthetic stereo frames; no device evidence\n";return 0;}catch(const std::exception &e){std::cerr<<e.what()<<"\n";return 1;}}
