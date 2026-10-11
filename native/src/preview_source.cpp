#include "riffine_preview.h"
#include "riffine_stream.h"
#include "riffine_decode.h"
#include <algorithm>
#include <cmath>
#include <cstdio>
#include <filesystem>
#include <memory>
#include <stdexcept>
#include <vector>
struct riffine_preview_source {
  riffine_decoder *decoder=nullptr;
  bool flushed=false,failed=false;
  riffine_stream *stream=nullptr;
  riffine_stream_limits limits{};
  uint64_t remaining=0,decoded=0,expected=0,emitted=0,discard=0,fade=0;
  uint32_t channels=0,rate=0,pending=0,offset=0,rawCapacity=0;
  std::vector<float> input,raw;
  ~riffine_preview_source() {
    riffine_stream_destroy(stream);
    riffine_decoder_close(decoder);
  }
};
namespace {
void require(bool ok,const char *message) { if(!ok) throw std::runtime_error(message); }
template<class F> int boundary(char *error,size_t size,F f) noexcept {
  if(error && size) error[0]=0;
  try {f();return 0;}
  catch(const std::exception &e) {if(error && size) std::snprintf(error,size,"%s",e.what());}
  catch(...) {if(error && size) std::snprintf(error,size,"unexpected preview source error");}
  return 1;
}
void dsp(int result,char *error) { if(result) throw std::runtime_error(error); }
}
int riffine_preview_open(const char *path,int64_t start,int64_t end,double speed,double pitch,
  riffine_preview_source **source,char *error,size_t size) {
  if(source) *source=nullptr;
  return boundary(error,size,[&] {
    require(path && source,"input and source result are required");
    require(start>=0 && end>start && std::isfinite(speed) && speed>=.25 && speed<=2 &&
      std::isfinite(pitch) && pitch>=-12 && pitch<=12,"invalid preview range or parameters");
    auto s=std::make_unique<riffine_preview_source>();
    require(riffine_decoder_open_file(path,&s->decoder,error,size)==0,"cannot open preview source");
    riffine_decoder_info sourceInfo{};
    require(riffine_decoder_info_get(s->decoder,&sourceInfo,error,size)==0 && uint64_t(end)<=sourceInfo.frames,"preview range exceeds source");
    s->channels=sourceInfo.channels;s->rate=sourceInfo.sample_rate;
    require(riffine_decoder_seek(s->decoder,uint64_t(start),error,size)==0,"cannot seek preview source");
    s->remaining=uint64_t(end-start);
    require(s->remaining<=(uint64_t(1)<<48),"preview range exceeds streaming frame counter limit");
    s->expected=uint64_t(std::llround(double(s->remaining)/speed));
    require(s->expected>0,"preview produces no output frames");
    riffine_stream_config sc{1,s->channels,s->rate,512,speed,pitch};char text[512];
    dsp(riffine_stream_create(&sc,&s->stream,text,sizeof(text)),text);
    dsp(riffine_stream_get_limits(s->stream,&s->limits,text,sizeof(text)),text);
    s->rawCapacity=std::max(s->limits.max_output_frames,s->limits.max_flush_frames);
    s->input.resize(512*s->channels);s->raw.resize(size_t(s->rawCapacity)*s->channels);
    s->discard=uint64_t(std::ceil(s->limits.input_latency_frames/speed))+s->limits.output_latency_frames;
    s->fade=std::min<uint64_t>(uint64_t(std::llround(s->rate*.005)),s->expected/2);
    *source=s.release();
  });
}
int riffine_preview_info_get(const riffine_preview_source *s,riffine_preview_info *info,char *error,size_t size) {
  return boundary(error,size,[&] {require(s && info,"source and info are required");*info={s->channels,s->rate,s->expected,s->decoded};});
}
int riffine_preview_read(riffine_preview_source *s,float *output,uint32_t capacity,uint32_t *produced,char *error,size_t size) {
  if(produced) *produced=0;
  return boundary(error,size,[&] {
    require(s && output && produced && capacity>0 && capacity<=8192,"invalid preview read buffers");
    require(!s->failed,"preview source failed; recreate it");
    s->failed=true;uint32_t written=0;char text[512];
    while(written<capacity && s->emitted<s->expected) {
      if(s->offset<s->pending) {
        auto skip=uint32_t(std::min<uint64_t>(s->pending-s->offset,s->discard));
        s->offset+=skip;s->discard-=skip;
        auto count=uint32_t(std::min<uint64_t>({capacity-written,s->pending-s->offset,s->expected-s->emitted}));
        for(uint32_t i=0;i<count;++i) {
          auto frame=s->emitted+i;
          double gain=s->fade ? std::min({1.,double(frame)/s->fade,double(s->expected-1-frame)/s->fade}) : 1.;
          for(uint32_t c=0;c<s->channels;++c)
            output[size_t(written+i)*s->channels+c]=float(s->raw[size_t(s->offset+i)*s->channels+c]*gain);
        }
        s->offset+=count;s->emitted+=count;written+=count;
      } else if(s->remaining) {
        auto count=uint32_t(std::min<uint64_t>(512,s->remaining));uint32_t read=0;
        require(riffine_decoder_read(s->decoder,s->input.data(),count,&read,error,size)==0 && read==count,"truncated preview source");
        uint32_t consumed=0;
        dsp(riffine_stream_process(s->stream,s->input.data(),uint32_t(read),s->raw.data(),s->rawCapacity,&consumed,&s->pending,text,sizeof(text)),text);
        require(consumed==read,"preview DSP did not consume block");
        s->remaining-=read;s->decoded+=read;s->offset=0;
      } else if(!s->flushed) {
        dsp(riffine_stream_flush(s->stream,s->raw.data(),s->rawCapacity,&s->pending,text,sizeof(text)),text);
        s->flushed=true;s->offset=0;
      } else {
        // Raw DSP counts floor fractional output. Match the project's rounded
        // duration with at most one zero-valued final frame (inside the fade).
        require(s->expected-s->emitted<=1,"preview DSP tail is incomplete");
        for(uint32_t c=0;c<s->channels;++c) output[size_t(written)*s->channels+c]=0;
        ++written;++s->emitted;
      }
    }
    s->failed=false;*produced=written;
  });
}
void riffine_preview_close(riffine_preview_source *source) {delete source;}
