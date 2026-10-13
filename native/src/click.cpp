#include "riffine_click.h"
#include <algorithm>
#include <cmath>
#include <cstdio>
#include <limits>

static int mix_window(uint32_t version, uint32_t channels,
 uint32_t rate, int64_t start, uint32_t frames, float level,
 const riffine_click_event *events, uint32_t count, float *output,
 int64_t window_end, char *error, size_t error_size) {
 auto reject = [&](const char *message) { if (error && error_size) std::snprintf(error,error_size,"%s",message); return 1; };
 if (version != 1 || channels < 1 || channels > 2 || rate < 8000 || rate > 192000 || start < 0 || frames > 8192 || start > std::numeric_limits<int64_t>::max()-frames || !std::isfinite(level) || level < 0 || level > 1 || window_end<start+frames || count > 256 || (frames && !output) || (count && !events)) return reject("invalid click mix version, format, block or level");
 for (uint32_t i=0;i<count;++i) {
  if (events[i].output_frame < 0 || (window_end!=std::numeric_limits<int64_t>::max()&&events[i].output_frame>=window_end) || events[i].accent > 1 || events[i].reserved != 0 || (i && events[i].output_frame < events[i-1].output_frame)) return reject("invalid or unordered click events");
 }
 for (uint32_t i=0;i<frames*channels;++i) if (!std::isfinite(output[i])) return reject("non-finite click mix input");
 // A disabled/empty click window leaves source samples exactly unchanged,
 // including existing over-range PCM. Clipping applies only to actual mixing.
 if (!level || !count) return 0;
 constexpr double pi=3.14159265358979323846;
 const int64_t tail=std::max<uint32_t>(1,rate/100), attack=std::max<uint32_t>(1,rate/2000);
 for (uint32_t frame=0;frame<frames;++frame) {
  const int64_t position=start+frame;
  double click=0;
  for (uint32_t i=0;i<count;++i) {
   if (events[i].output_frame > position) break;
   const int64_t age=position-events[i].output_frame;
   const int64_t duration=std::min(tail,window_end-events[i].output_frame);
   if (age>=duration) continue;
   const double envelope=std::cos(pi*.5*static_cast<double>(age)/duration);
   const double ramp=std::min(1.0,static_cast<double>(age)/attack);
   const double frequency=events[i].accent ? 1500.0 : 1000.0;
   const double gain=(events[i].accent ? .25 : .18)*level;
   click+=gain*ramp*envelope*envelope*std::sin(2*pi*frequency*static_cast<double>(age)/rate);
  }
  if (click!=0) for(uint32_t channel=0;channel<channels;++channel) {
   const auto index=frame*channels+channel;
   output[index]=static_cast<float>(std::clamp(static_cast<double>(output[index])+click,-1.0,1.0));
  }
 }
 return 0;
}

extern "C" int riffine_click_mix(uint32_t version,uint32_t channels,uint32_t rate,
 int64_t start,uint32_t frames,float level,const riffine_click_event *events,uint32_t count,
 float *output,char *error,size_t error_size){
 return mix_window(version,channels,rate,start,frames,level,events,count,output,std::numeric_limits<int64_t>::max(),error,error_size);
}
extern "C" int riffine_click_mix_window(uint32_t version,uint32_t channels,uint32_t rate,
 int64_t start,uint32_t frames,float level,const riffine_click_event *events,uint32_t count,
 float *output,int64_t window_end,char *error,size_t error_size){
 return mix_window(version,channels,rate,start,frames,level,events,count,output,window_end,error,error_size);
}
