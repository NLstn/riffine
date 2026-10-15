#include "riffine_selection.h"
#include "riffine_stream.h"
#include <algorithm>
#include <cmath>
#include <cstdio>
#include <memory>
#include <stdexcept>
#include <vector>

struct riffine_selection {
 riffine_stream *stream=nullptr;
 riffine_stream_limits limits{};
 uint64_t sourceFrames=0,remaining=0,expected=0,emitted=0,discard=0,fade=0;
 uint32_t channels=0,rate=0,pending=0,offset=0,rawCapacity=0;
 bool flushed=false,failed=false,configured=false;
 std::vector<float> raw;
 ~riffine_selection() {riffine_stream_destroy(stream);}
};
namespace {
void require(bool ok,const char *message) {if(!ok) throw std::runtime_error(message);}
template<class F> int boundary(char *error,size_t size,F f) noexcept {
 if(error && size) error[0]=0;
 try {f();return 0;}
 catch(const std::exception &e) {if(error && size) std::snprintf(error,size,"%s",e.what());}
 catch(...) {if(error && size) std::snprintf(error,size,"unexpected selection processing error");}
 return 1;
}
void dsp(int result,char *error) {if(result) throw std::runtime_error(error);}
}
int riffine_selection_create(uint32_t channels,uint32_t rate,uint64_t frames,double speed,double pitch,
 riffine_selection **selection,char *error,size_t size) {
 if(selection) *selection=nullptr;
 return boundary(error,size,[&] {
  require(selection && frames>0 && frames<=(uint64_t(1)<<48),"invalid selection frame count");
  auto s=std::make_unique<riffine_selection>();
  riffine_stream_config config{1,channels,rate,512,speed,pitch};char text[512];
  dsp(riffine_stream_create(&config,&s->stream,text,sizeof(text)),text);
  dsp(riffine_stream_get_limits(s->stream,&s->limits,text,sizeof(text)),text);
  s->channels=channels;s->rate=rate;s->sourceFrames=s->remaining=frames;
  s->expected=uint64_t(std::llround(double(frames)/speed));
  require(s->expected>0,"selection produces no output frames");
  s->rawCapacity=std::max(s->limits.max_output_frames,s->limits.max_flush_frames);
  s->raw.resize(size_t(s->rawCapacity)*channels);
  s->discard=uint64_t(std::ceil(s->limits.input_latency_frames/speed))+s->limits.output_latency_frames;
  s->fade=std::min<uint64_t>(uint64_t(std::llround(rate*.005)),s->expected/2);
  *selection=s.release();
 });
}
int riffine_selection_set_edge_fade_frames(riffine_selection *s,uint64_t frames,char *error,size_t size) {
 return boundary(error,size,[&] {
  require(s && !s->failed,"selection must be usable");
  require(s->remaining==s->sourceFrames && s->emitted==0 && !s->flushed,"edge fade requires an unprocessed selection");
  require(frames<=(s->expected+1)/2,"edge fade exceeds selection bound");
  s->fade=frames;
 });
}
int riffine_selection_info_get(const riffine_selection *s,riffine_selection_info *info,char *error,size_t size) {
 return boundary(error,size,[&] {require(s && info,"selection and info are required");*info={s->channels,s->rate,s->expected,s->sourceFrames-s->remaining};});
}
int riffine_selection_configure(riffine_selection *s,double speed,double pitch,riffine_selection_change *change,char *error,size_t size) {
 if(error&&size)error[0]=0;
 if(!s||!change||s->failed||!std::isfinite(speed)||speed<.25||speed>2||!std::isfinite(pitch)||pitch<-12||pitch>12)
  return boundary(error,size,[&]{require(false,"invalid live selection parameters");});
 if(s->flushed||s->discard||!s->emitted||!s->remaining||s->offset!=s->pending||s->sourceFrames-s->remaining<=s->limits.input_latency_frames) return 2;
 return boundary(error,size,[&]{
  // The synthesis buffer already holds outputLatency() old-parameter frames.
  // Input latency is in source frames; do not reinterpret buffered source as
  // already heard when changing speed. Both anchors survive repeated updates.
  auto source=s->sourceFrames-s->remaining-s->limits.input_latency_frames;
  auto output=s->emitted+s->limits.output_latency_frames;
  auto expected=output+uint64_t(std::llround(double(s->sourceFrames-source)/speed));
  require(expected>output&&expected<=(uint64_t(1)<<50),"live output timeline exceeds bound");
  char text[512];dsp(riffine_stream_configure(s->stream,speed,pitch,text,sizeof(text)),text);
  s->expected=expected;s->configured=true;*change={output,source,expected};
 });
}
int riffine_selection_push(riffine_selection *s,const float *input,uint32_t frames,char *error,size_t size) {
 return boundary(error,size,[&] {
  require(s && input && !s->failed,"selection and input must be usable");
  require(frames>0 && frames<=512 && frames<=s->remaining && s->offset==s->pending,"selection cannot accept input yet");
  for(size_t i=0;i<size_t(frames)*s->channels;++i) require(std::isfinite(input[i]),"non-finite selection input");
  s->failed=true;uint32_t consumed=0;char text[512];
  dsp(riffine_stream_process(s->stream,input,frames,s->raw.data(),s->rawCapacity,&consumed,&s->pending,text,sizeof(text)),text);
  require(consumed==frames,"selection DSP did not consume block");
  s->remaining-=frames;s->offset=0;s->failed=false;
 });
}
int riffine_selection_read(riffine_selection *s,float *output,uint32_t capacity,uint32_t *produced,uint32_t *needsInput,char *error,size_t size) {
 if(produced) *produced=0;
 if(needsInput) *needsInput=0;
 return boundary(error,size,[&] {
  require(s && output && produced && needsInput && produced!=needsInput && capacity>0 && capacity<=8192,"invalid selection read buffers");
  require(!s->failed,"selection failed; recreate it");
  s->failed=true;uint32_t written=0;char text[512];
  while(written<capacity && s->emitted<s->expected) {
   if(s->offset<s->pending) {
    auto skip=uint32_t(std::min<uint64_t>(s->pending-s->offset,s->discard));
    s->offset+=skip;s->discard-=skip;
    auto count=uint32_t(std::min<uint64_t>({capacity-written,s->pending-s->offset,s->expected-s->emitted}));
    for(uint32_t i=0;i<count;++i) {
     auto frame=s->emitted+i;
     double gain=s->fade ? std::min({1.,double(frame)/s->fade,double(s->expected-1-frame)/s->fade}) : 1.;
     for(uint32_t c=0;c<s->channels;++c) output[size_t(written+i)*s->channels+c]=float(s->raw[size_t(s->offset+i)*s->channels+c]*gain);
    }
    s->offset+=count;s->emitted+=count;written+=count;
   } else if(s->remaining) {
    *needsInput=1;break;
   } else if(!s->flushed) {
    dsp(riffine_stream_flush(s->stream,s->raw.data(),s->rawCapacity,&s->pending,text,sizeof(text)),text);
    s->flushed=true;s->offset=0;
   } else {
    require(s->expected-s->emitted<=(s->configured ? 2u : 1u),"selection DSP tail is incomplete");
    for(uint32_t c=0;c<s->channels;++c) output[size_t(written)*s->channels+c]=0;
    ++written;++s->emitted;
   }
  }
  s->failed=false;*produced=written;
 });
}
void riffine_selection_destroy(riffine_selection *s) {delete s;}
