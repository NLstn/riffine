#include "riffine_rate_selection.h"
#include "riffine_selection.h"
#define NOMINMAX
#include "miniaudio.h"
#include <algorithm>
#include <array>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <memory>
#include <stdexcept>
struct riffine_rate_selection {
 riffine_selection *inner=nullptr;ma_linear_resampler resampler{};
 riffine_rate_selection_info info{};
 uint64_t source_frames=0,emitted=0,discard=0,fade=0;
 uint32_t pending_input=0,pending_output=0,offset=0,tail=0;
 bool resampler_ready=false,inner_ended=false,failed=false,waiting=false;
 std::array<float,2048> input{};
 std::array<float,8192*2> output{};
 ~riffine_rate_selection(){if(resampler_ready)ma_linear_resampler_uninit(&resampler,nullptr);riffine_selection_destroy(inner);}
};
namespace {
void require(bool b,const char *text){if(!b)throw std::runtime_error(text);}
void ok(int result,const char *text){if(result)throw std::runtime_error(text);}
template<class F>int boundary(char *error,size_t size,F f)noexcept{
 if(error&&size)error[0]=0;try{f();return 0;}catch(const std::exception &e){if(error&&size)std::snprintf(error,size,"%s",e.what());}catch(...){if(error&&size)std::snprintf(error,size,"unexpected rate processing error");}return 1;
}
}
int riffine_rate_selection_create(uint32_t channels,uint32_t sourceRate,uint32_t outputRate,uint64_t frames,double speed,double pitch,riffine_rate_selection **result,char *error,size_t size){
 if(result)*result=nullptr;return boundary(error,size,[&]{
  require(result&&outputRate>=8000&&outputRate<=192000,"invalid device output rate/result");
  auto s=std::make_unique<riffine_rate_selection>();char text[512];
  ok(riffine_selection_create(channels,sourceRate,frames,speed,pitch,&s->inner,text,sizeof(text)),text);
  ok(riffine_selection_set_edge_fade_frames(s->inner,0,text,sizeof(text)),text);
  auto expected=std::max<uint64_t>(1,uint64_t(std::llround(double(frames)*outputRate/sourceRate/speed)));
  s->info={channels,sourceRate,outputRate,0,expected,0};s->source_frames=frames;
  s->fade=std::min<uint64_t>(uint64_t(std::llround(outputRate*.005)),expected/2);
  if(sourceRate!=outputRate){auto config=ma_linear_resampler_config_init(ma_format_f32,channels,sourceRate,outputRate);
   require(ma_linear_resampler_init(&config,nullptr,&s->resampler)==MA_SUCCESS,"cannot create rate converter");s->resampler_ready=true;
   s->discard=ma_linear_resampler_get_output_latency(&s->resampler);require(s->discard<=8192,"rate latency exceeds bounded output");s->info.resampler_latency_frames=uint32_t(s->discard);
  }
  *result=s.release();
 });
}
int riffine_rate_selection_info_get(const riffine_rate_selection *s,riffine_rate_selection_info *info,char *error,size_t size){
 return boundary(error,size,[&]{require(s&&info,"selection/info required");riffine_selection_info inner{};char text[512];ok(riffine_selection_info_get(s->inner,&inner,text,sizeof(text)),text);*info=s->info;info->consumed_source_frames=inner.consumed_source_frames;});
}
int riffine_rate_selection_set_edge_fade_frames(riffine_rate_selection *s,uint64_t frames,char *error,size_t size){
 return boundary(error,size,[&]{require(s&&!s->failed&&!s->emitted&&!s->pending_input&&!s->pending_output,"fade requires unprocessed selection");riffine_selection_info info{};char text[512];ok(riffine_selection_info_get(s->inner,&info,text,sizeof(text)),text);require(!info.consumed_source_frames&&frames<=(s->info.output_frames+1)/2,"invalid scheduled device fade");s->fade=frames;});
}
int riffine_rate_selection_push(riffine_rate_selection *s,const float *input,uint32_t frames,char *error,size_t size){
 return boundary(error,size,[&]{require(s&&!s->failed&&s->waiting,"device selection does not need source input");char text[512];ok(riffine_selection_push(s->inner,input,frames,text,sizeof(text)),text);s->waiting=false;});
}
int riffine_rate_selection_read(riffine_rate_selection *s,float *output,uint32_t capacity,uint32_t *produced,uint32_t *needsInput,char *error,size_t size){
 if(produced)*produced=0;if(needsInput)*needsInput=0;return boundary(error,size,[&]{
  require(s&&output&&produced&&needsInput&&produced!=needsInput&&capacity&&capacity<=8192&&!s->failed,"invalid device selection read");
  s->failed=true;uint32_t written=0;char text[512];
  while(written<capacity&&s->emitted<s->info.output_frames){
   if(s->offset<s->pending_output){
    auto skip=uint32_t(std::min<uint64_t>(s->pending_output-s->offset,s->discard));s->offset+=skip;s->discard-=skip;
    auto count=uint32_t(std::min<uint64_t>({capacity-written,s->pending_output-s->offset,s->info.output_frames-s->emitted}));
    for(uint32_t i=0;i<count;++i){auto frame=s->emitted+i;double gain=s->fade?std::min({1.,double(frame)/s->fade,double(s->info.output_frames-1-frame)/s->fade}):1.;
     for(uint32_t c=0;c<s->info.channels;++c){auto value=float(s->output[size_t(s->offset+i)*s->info.channels+c]*gain);require(std::isfinite(value),"non-finite resampled output");output[size_t(written+i)*s->info.channels+c]=value;}
    }
    s->offset+=count;s->emitted+=count;written+=count;continue;
   }
   s->pending_output=s->offset=0;
   if(s->pending_input){
    if(s->resampler_ready){ma_uint64 input=s->pending_input,out=8192;
     require(ma_linear_resampler_process_pcm_frames(&s->resampler,s->input.data(),&input,s->output.data(),&out)==MA_SUCCESS,"rate processing failed");
     require(input<=s->pending_input&&out<=8192,"rate processor exceeded bounds");
     s->pending_input-=uint32_t(input);if(s->pending_input)std::memmove(s->input.data(),s->input.data()+size_t(input)*s->info.channels,size_t(s->pending_input)*s->info.channels*sizeof(float));
     s->pending_output=uint32_t(out);if(input||out)continue;
    }else{
     s->pending_output=s->pending_input;std::copy_n(s->input.data(),size_t(s->pending_input)*s->info.channels,s->output.data());s->pending_input=0;continue;
    }
   }
   require(s->pending_input<1024,"rate processor made no progress");
   if(!s->inner_ended){uint32_t count=0,need=0;
    ok(riffine_selection_read(s->inner,s->input.data()+size_t(s->pending_input)*s->info.channels,std::min(512u,1024-s->pending_input),&count,&need,text,sizeof(text)),text);
    s->pending_input+=count;
    if(count)continue;
    if(need){s->waiting=true;*needsInput=1;break;}
    s->inner_ended=true;
   }
   // Fixed finite zero tail drains converter latency and rounding. Never repeat
   // source samples or grow storage to the selection duration.
   require(s->tail<4096,"rate converter tail exceeds bounded input");
   auto count=std::min({512u,1024-s->pending_input,4096-s->tail});
   std::fill_n(s->input.data()+size_t(s->pending_input)*s->info.channels,size_t(count)*s->info.channels,0.f);
   s->pending_input+=count;s->tail+=count;
  }
  if(s->emitted==s->info.output_frames){riffine_selection_info info{};ok(riffine_selection_info_get(s->inner,&info,text,sizeof(text)),text);require(info.consumed_source_frames==s->source_frames,"device output ended before source consumption");}
  s->failed=false;*produced=written;
 });
}
void riffine_rate_selection_destroy(riffine_rate_selection *s){delete s;}
