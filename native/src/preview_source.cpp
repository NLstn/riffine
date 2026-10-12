#include "riffine_preview.h"
#include "riffine_selection.h"
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
 riffine_selection *selection=nullptr;
 uint64_t remaining=0,decoded=0,expected=0;
 uint32_t channels=0,rate=0;
 bool failed=false;
 std::vector<float> input;
 ~riffine_preview_source() { riffine_selection_destroy(selection); riffine_decoder_close(decoder); }
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
static int previewOpen(const char *path,uint64_t handle,bool borrowed,int64_t start,int64_t end,double speed,double pitch,
  riffine_preview_source **source,char *error,size_t size) {
  if(source) *source=nullptr;
  return boundary(error,size,[&] {
    require((borrowed || path) && source,"input and source result are required");
    require(start>=0 && end>start && std::isfinite(speed) && speed>=.25 && speed<=2 &&
      std::isfinite(pitch) && pitch>=-12 && pitch<=12,"invalid preview range or parameters");
    auto s=std::make_unique<riffine_preview_source>();
    auto result=borrowed ? riffine_decoder_open_handle(handle,&s->decoder,error,size) : riffine_decoder_open_file(path,&s->decoder,error,size);
    require(result==0,"cannot open preview source");
    riffine_decoder_info sourceInfo{};
    require(riffine_decoder_info_get(s->decoder,&sourceInfo,error,size)==0 && uint64_t(end)<=sourceInfo.frames,"preview range exceeds source");
    s->channels=sourceInfo.channels;s->rate=sourceInfo.sample_rate;
    require(riffine_decoder_seek(s->decoder,uint64_t(start),error,size)==0,"cannot seek preview source");
    require(riffine_decoder_info_get(s->decoder,&sourceInfo,error,size)==0,"cannot query seek decode diagnostics");s->decoded=sourceInfo.decoded_frames;
    s->remaining=uint64_t(end-start);
    require(s->remaining<=(uint64_t(1)<<48),"preview range exceeds streaming frame counter limit");
    char text[512];
    dsp(riffine_selection_create(s->channels,s->rate,s->remaining,speed,pitch,&s->selection,text,sizeof(text)),text);
    riffine_selection_info selectionInfo{};
    dsp(riffine_selection_info_get(s->selection,&selectionInfo,text,sizeof(text)),text);
    s->expected=selectionInfo.output_frames;
    s->input.resize(512*s->channels);
    *source=s.release();
  });
}
int riffine_preview_open(const char *path,int64_t start,int64_t end,double speed,double pitch,riffine_preview_source **source,char *error,size_t size){
 return previewOpen(path,0,false,start,end,speed,pitch,source,error,size);
}
int riffine_preview_open_handle(uint64_t handle,int64_t start,int64_t end,double speed,double pitch,riffine_preview_source **source,char *error,size_t size){
 return previewOpen(nullptr,handle,true,start,end,speed,pitch,source,error,size);
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
    while(written<capacity) {
      uint32_t count=0,needsInput=0;
      dsp(riffine_selection_read(s->selection,output+size_t(written)*s->channels,capacity-written,&count,&needsInput,text,sizeof(text)),text);
      written+=count;
      if(!needsInput) break;
      // If the requested output block is already full, defer decoding until the
      // next read. Source/processing buffers remain fixed at construction.
      if(written==capacity) break;
      auto frames=uint32_t(std::min<uint64_t>(512,s->remaining));uint32_t read=0;
      require(riffine_decoder_read(s->decoder,s->input.data(),frames,&read,error,size)==0 && read==frames,"truncated preview source");
      dsp(riffine_selection_push(s->selection,s->input.data(),read,text,sizeof(text)),text);
      s->remaining-=read;s->decoded+=read;
    }
    s->failed=false;*produced=written;
  });
}
void riffine_preview_close(riffine_preview_source *source) {delete source;}
