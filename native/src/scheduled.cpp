#include "riffine_scheduled.h"
#include "riffine_decode.h"
#include "riffine_selection.h"
#include "riffine_rate_selection.h"
#include <algorithm>
#include <array>
#include <cmath>
#include <cstdio>
#include <memory>
#include <stdexcept>

struct ScheduledSlot {
 riffine_decoder *decoder=nullptr;
 riffine_rate_selection *selection=nullptr;
 riffine_schedule schedule{};
 uint64_t remaining=0,generated=0,cue_offset=0;
 uint32_t cue_kind=0,click_count=0;
 float click_level=0;
 std::array<riffine_click_event,32> clicks{};
 std::array<float,1024> input{};
 ~ScheduledSlot(){clear();riffine_decoder_close(decoder);}
 void clear(){riffine_rate_selection_destroy(selection);selection=nullptr;remaining=generated=cue_offset=0;cue_kind=click_count=0;click_level=0;}
};
struct riffine_scheduled_worker {
 std::array<ScheduledSlot,2> slots;
 riffine_scheduled_info info{1,0,0,0,0,0,0};
 unsigned front=0;
 uint64_t next_output=0;
 uint32_t output_rate=0;
 bool failed=false;
};
namespace {
void require(bool ok,const char *message){if(!ok)throw std::runtime_error(message);}
void ok(int result,const char *message){if(result)throw std::runtime_error(message);}
template<class F>int boundary(char *error,size_t size,F f) noexcept {
 if(error&&size)error[0]=0;
 try {f();return 0;}
 catch(const std::exception &e){if(error&&size)std::snprintf(error,size,"%s",e.what());}
 catch(...){if(error&&size)std::snprintf(error,size,"unexpected scheduled worker error");}
 return 1;
}
}
int riffine_scheduled_open(const char *path,riffine_scheduled_worker **result,char *error,size_t size){
 if(result)*result=nullptr;
 return boundary(error,size,[&]{
  require(path&&result,"source path and worker result required");
  auto worker=std::make_unique<riffine_scheduled_worker>();char text[512];
  for(auto &slot:worker->slots){
   ok(riffine_decoder_open_file(path,&slot.decoder,text,sizeof(text)),text);
   riffine_decoder_info info{};ok(riffine_decoder_info_get(slot.decoder,&info,text,sizeof(text)),text);
   if(!worker->info.channels){worker->info.channels=info.channels;worker->info.sample_rate=info.sample_rate;worker->info.source_frames=info.frames;}
   require(info.channels==worker->info.channels&&info.sample_rate==worker->info.sample_rate&&info.frames==worker->info.source_frames,"source changed while opening scheduled cursors");
  }
  worker->output_rate=worker->info.sample_rate;*result=worker.release();
 });
}
int riffine_scheduled_open_handle(uint64_t handle,riffine_scheduled_worker **result,char *error,size_t size){
 if(result)*result=nullptr;
 return boundary(error,size,[&]{
  require(result,"worker result required");auto worker=std::make_unique<riffine_scheduled_worker>();char text[512];
  for(auto &slot:worker->slots){
   ok(riffine_decoder_open_handle(handle,&slot.decoder,text,sizeof(text)),text);
   riffine_decoder_info info{};ok(riffine_decoder_info_get(slot.decoder,&info,text,sizeof(text)),text);
   if(!worker->info.channels){worker->info.channels=info.channels;worker->info.sample_rate=info.sample_rate;worker->info.source_frames=info.frames;}
   require(info.channels==worker->info.channels&&info.sample_rate==worker->info.sample_rate&&info.frames==worker->info.source_frames,"source changed while opening borrowed cursors");
  }
  worker->output_rate=worker->info.sample_rate;*result=worker.release();
 });
}
int riffine_scheduled_set_output_rate(riffine_scheduled_worker *w,uint32_t rate,char *error,size_t size){
 return boundary(error,size,[&]{require(w&&!w->failed&&!w->info.generation&&!w->info.pending,"output rate requires an unused worker");require(rate>=8000&&rate<=192000,"invalid output sample rate");w->output_rate=rate;});
}
int riffine_scheduled_output_rate_get(const riffine_scheduled_worker *w,uint32_t *rate,char *error,size_t size){
 return boundary(error,size,[&]{require(w&&rate,"worker/output rate required");*rate=w->output_rate;});
}
int riffine_scheduled_info_get(const riffine_scheduled_worker *w,riffine_scheduled_info *result,char *error,size_t size){
 return boundary(error,size,[&]{require(w&&result,"worker and info required");*result=w->info;});
}
int riffine_scheduled_reset(riffine_scheduled_worker *w,uint64_t generation,char *error,size_t size){
 return boundary(error,size,[&]{
  require(w&&!w->failed,"worker failed or missing; recreate it");
  require(generation>w->info.generation,"generation must increase");
  for(auto &slot:w->slots)slot.clear();
  w->front=0;w->info.pending=0;w->info.generation=generation;w->next_output=0;
 });
}
int riffine_scheduled_submit(riffine_scheduled_worker *w,const riffine_schedule *schedule,char *error,size_t size){
 return boundary(error,size,[&]{
  require(w&&!w->failed&&schedule,"worker failed or missing schedule");
  const auto &s=*schedule;
  require(w->info.generation&&s.generation==w->info.generation&&s.sequence>w->info.last_sequence,"invalid generation or sequence");
  require(w->info.pending<2,"two selections already pending");
  require(s.start_frame<s.end_frame&&s.end_frame<=w->info.source_frames,"invalid scheduled source range");
  require(std::isfinite(s.speed)&&s.speed>=.25&&s.speed<=2&&std::isfinite(s.semitones)&&s.semitones>=-12&&s.semitones<=12,"invalid scheduled parameters");
  auto expected=uint64_t(std::llround(double(s.end_frame-s.start_frame)*w->output_rate/w->info.sample_rate/s.speed));
  expected=std::max<uint64_t>(1,expected);
  require(s.output_start==w->next_output&&s.output_end>s.output_start&&s.output_end-s.output_start==expected&&s.output_end<=uint64_t(INT64_MAX),"invalid scheduled output coordinates");
  require(s.edge_fade_frames<=(expected+1)/2,"invalid scheduled fade");
  auto &slot=w->slots[(w->front+w->info.pending)%2];
  char text[512];riffine_rate_selection *selection=nullptr;
  ok(riffine_rate_selection_create(w->info.channels,w->info.sample_rate,w->output_rate,s.end_frame-s.start_frame,s.speed,s.semitones,&selection,text,sizeof(text)),text);
  std::unique_ptr<riffine_rate_selection,decltype(&riffine_rate_selection_destroy)> owner(selection,riffine_rate_selection_destroy);
  ok(riffine_rate_selection_set_edge_fade_frames(selection,s.edge_fade_frames,text,sizeof(text)),text);
  // Seek can fail after touching decoder state, so runtime failure poisons the
  // worker. All schema/range/capacity validation above precedes this boundary.
  w->failed=true;
  ok(riffine_decoder_seek(slot.decoder,s.start_frame,text,sizeof(text)),text);
  slot.selection=owner.release();slot.schedule=s;slot.remaining=s.end_frame-s.start_frame;slot.generated=0;
  ++w->info.pending;w->info.last_sequence=s.sequence;w->next_output=s.output_end;w->failed=false;
 });
}
int riffine_scheduled_submit_cue(riffine_scheduled_worker *w,const riffine_schedule *schedule,
 uint32_t kind,uint64_t offset,float level,const riffine_click_event *events,uint32_t count,char *error,size_t size){
 return boundary(error,size,[&]{
  require(w&&!w->failed&&schedule,"worker failed or missing cue schedule");const auto &s=*schedule;
  require(w->info.generation&&s.generation==w->info.generation&&s.sequence>w->info.last_sequence&&w->info.pending<2,"invalid cue generation, sequence or queue capacity");
  require((kind==1||kind==2)&&s.start_frame==s.end_frame&&s.start_frame<w->info.source_frames&&s.edge_fade_frames==0,"invalid cue phase or source anchor");
  require(std::isfinite(s.speed)&&s.speed>=.25&&s.speed<=2&&std::isfinite(s.semitones)&&s.semitones>=-12&&s.semitones<=12,"invalid cue parameters");
  require(s.output_start==w->next_output&&s.output_end>s.output_start&&s.output_end<=uint64_t(INT64_MAX),"invalid cue output coordinates");
  const auto length=s.output_end-s.output_start;
  require(offset<=uint64_t(w->output_rate)*120&&length<=uint64_t(w->output_rate)*120-offset,"cue duration exceeds 120 seconds");
  require(std::isfinite(level)&&level>=0&&level<=1&&count<=32&&(!count||events),"invalid cue click controls");
  require(kind!=2||(!count&&level==0),"rest phases cannot contain clicks");
  for(uint32_t i=0;i<count;++i)require(events[i].output_frame>=0&&uint64_t(events[i].output_frame)<offset+length&&events[i].accent<=1&&!events[i].reserved&&(!i||events[i].output_frame>events[i-1].output_frame),"invalid bounded cue events");
  auto &slot=w->slots[(w->front+w->info.pending)%2];slot.clear();slot.schedule=s;
  slot.cue_kind=kind;slot.cue_offset=offset;slot.click_level=level;slot.click_count=count;
  if(count)std::copy(events,events+count,slot.clicks.begin());
  ++w->info.pending;w->info.last_sequence=s.sequence;w->next_output=s.output_end;
 });
}
int riffine_scheduled_read(riffine_scheduled_worker *w,float *output,uint32_t capacity,riffine_scheduled_block *result,char *error,size_t size){
 if(result)*result={};
 return boundary(error,size,[&]{
  require(w&&!w->failed&&output&&result&&capacity&&capacity<=8192,"invalid read or failed worker");
  if(!w->info.pending)return;
  auto &slot=w->slots[w->front];char text[512];uint32_t written=0;
  w->failed=true;
  if(slot.cue_kind){
   written=uint32_t(std::min<uint64_t>(capacity,slot.schedule.output_end-slot.schedule.output_start-slot.generated));
   std::fill(output,output+size_t(written)*w->info.channels,0.0f);
   if(slot.cue_kind==1)ok(riffine_click_mix_window(1,w->info.channels,w->output_rate,int64_t(slot.cue_offset+slot.generated),written,slot.click_level,slot.clicks.data(),slot.click_count,output,int64_t(slot.cue_offset+slot.schedule.output_end-slot.schedule.output_start),text,sizeof(text)),text);
  }else while(written<capacity){
   uint32_t count=0,needs_input=0;
   ok(riffine_rate_selection_read(slot.selection,output+size_t(written)*w->info.channels,capacity-written,&count,&needs_input,text,sizeof(text)),text);
   written+=count;
   if(!needs_input||written==capacity)break;
   auto frames=uint32_t(std::min<uint64_t>(512,slot.remaining));uint32_t read=0;
   require(frames>0,"processing requested source beyond schedule");
   ok(riffine_decoder_read(slot.decoder,slot.input.data(),frames,&read,text,sizeof(text)),text);
   require(read==frames,"truncated scheduled source");
   ok(riffine_rate_selection_push(slot.selection,slot.input.data(),read,text,sizeof(text)),text);
   slot.remaining-=read;
  }
  auto expected=slot.schedule.output_end-slot.schedule.output_start;
  require(written&&written<=expected-slot.generated,"scheduled output length mismatch");
  riffine_scheduled_block block{slot.schedule.generation,slot.schedule.sequence,slot.schedule.output_start+slot.generated,written,0};
  slot.generated+=written;
  if(slot.generated==expected){require(!slot.remaining,"scheduled output ended before source consumption");block.generated_complete=1;slot.clear();--w->info.pending;w->front=(w->front+1)%2;}
  w->failed=false;*result=block;
 });
}
void riffine_scheduled_close(riffine_scheduled_worker *worker){delete worker;}
