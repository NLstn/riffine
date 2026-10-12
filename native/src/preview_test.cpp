#include "riffine_preview.h"
#include "riffine_audio.h"
#include "riffine_decode.h"
#include "riffine_preview_queue.h"
#include <chrono>
#include <algorithm>
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <filesystem>
#include <memory>
#include <new>
#include <stdexcept>
#include <string>
#include <vector>
static bool trackAllocations=false;
static unsigned allocations=0;
static size_t largestAllocation=0;
void *operator new(size_t size) {
  if(trackAllocations) {++allocations;largestAllocation=std::max(largestAllocation,size);}
  if(void *p=std::malloc(size ? size : 1)) return p;
  throw std::bad_alloc();
}
void *operator new[](size_t size) {return ::operator new(size);}
void operator delete(void *p) noexcept {std::free(p);}
void operator delete[](void *p) noexcept {std::free(p);}
void operator delete(void *p,size_t) noexcept {std::free(p);}
void operator delete[](void *p,size_t) noexcept {std::free(p);}
namespace {
void check(bool ok,const char *message) {if(!ok) throw std::runtime_error(message);}
char error[512];
void ok(int result) {if(result) throw std::runtime_error(error);}
struct Temp {
  std::filesystem::path path=std::filesystem::temp_directory_path()/
    ("riffine-preview-"+std::to_string(std::chrono::steady_clock::now().time_since_epoch().count()));
  Temp() {check(std::filesystem::create_directory(path),"cannot create test directory");}
  ~Temp() {std::error_code ec;std::filesystem::remove_all(path,ec);}
};
void little(FILE *file,uint64_t n,unsigned bytes) {
  for(unsigned i=0;i<bytes;++i) check(std::fputc(int((n>>(i*8))&255),file)!=EOF,"fixture write failed");
}
void wav(const std::filesystem::path &path,unsigned channels,unsigned bits,unsigned rate,unsigned seconds,bool sparse=false) {
#ifdef _WIN32
  FILE *raw=_wfopen(path.c_str(),L"wbx");
#else
  FILE *raw=std::fopen(path.c_str(),"wbx");
#endif
  check(raw,"cannot exclusively create fixture");
  std::unique_ptr<FILE,decltype(&std::fclose)> file(raw,std::fclose);
  auto bytes=uint64_t(rate)*seconds*channels*bits/8;check(bytes<UINT32_MAX-36,"fixture too large");
  check(std::fwrite("RIFF",1,4,raw)==4,"fixture header");little(raw,36+bytes,4);
  check(std::fwrite("WAVEfmt ",1,8,raw)==8,"fixture header");little(raw,16,4);little(raw,1,2);little(raw,channels,2);
  little(raw,rate,4);little(raw,rate*channels*bits/8,4);little(raw,channels*bits/8,2);little(raw,bits,2);
  check(std::fwrite("data",1,4,raw)==4,"fixture header");little(raw,bytes,4);
  auto generated=uint64_t(rate)*(sparse ? 1 : seconds);
  for(uint64_t i=0;i<generated;++i) for(unsigned c=0;c<channels;++c) {
    double hz=(c ? 660 : 440)*(i>=rate && i<2*rate ? 2 : 1);
    auto sample=int64_t(std::llround(.2*std::sin(2*3.141592653589793*hz*double(i)/rate)*double((uint64_t(1)<<(bits-1))-1)));
    little(raw,uint64_t(sample),bits/8);
  }
  if(sparse) {
#ifdef _WIN32
    check(_fseeki64(raw,int64_t(44+bytes-1),SEEK_SET)==0,"fixture seek");
#else
    check(fseeko(raw,int64_t(44+bytes-1),SEEK_SET)==0,"fixture seek");
#endif
    check(std::fputc(0,raw)!=EOF,"fixture sparse tail");
  }
  check(std::fflush(raw)==0,"fixture flush");
}
using Source=std::unique_ptr<riffine_preview_source,decltype(&riffine_preview_close)>;
Source open(const std::filesystem::path &path,int64_t start,int64_t end,double speed,double pitch) {
  riffine_preview_source *source=nullptr;
  auto name=path.u8string();ok(riffine_preview_open(name.c_str(),start,end,speed,pitch,&source,error,sizeof(error)));
  return Source(source,riffine_preview_close);
}
void signal(const std::filesystem::path &path,unsigned channels,unsigned rate,double speed,double pitch) {
  // Select only the doubled-frequency middle second, not its neighbours.
  auto source=open(path,rate,rate*2,speed,pitch);
  riffine_preview_info info{};ok(riffine_preview_info_get(source.get(),&info,error,sizeof(error)));
  check(info.channels==channels && info.sample_rate==rate && !info.decoded_frames,"open decoded PCM eagerly");
  std::vector<float> block(8192*channels);uint32_t count=99;
  check(riffine_preview_read(source.get(),block.data(),0,&count,error,sizeof(error)) && !count,"invalid read accepted");
  uint64_t frames=0,measured=0,crossings[2]{};float previous[2]{};double energy[2]{};
  allocations=0;trackAllocations=true;
  for(;;) {
    ok(riffine_preview_read(source.get(),block.data(),113,&count,error,sizeof(error)));
    if(!count) break;
    for(unsigned i=0;i<count;++i) {
      bool measure=frames+i>rate/10 && frames+i+rate/10<info.output_frames;
      for(unsigned c=0;c<channels;++c) {
        auto sample=block[size_t(i)*channels+c];check(std::isfinite(sample),"nonfinite preview");
        if(measure) {if(previous[c]<=0 && sample>0) ++crossings[c];energy[c]+=double(sample)*sample;}
        previous[c]=sample;
      }
      if(measure) ++measured;
    }
    frames+=count;
  }
  trackAllocations=false;check(!allocations,"preview cursor grew C++ storage during reads");
  check(frames==uint64_t(std::llround(rate/speed)),"wrong preview duration");
  check(measured>0,"no measured signal");
  for(unsigned c=0;c<channels;++c) {
    double expected=(c ? 1320 : 880)*std::pow(2,pitch/12);
    double hz=double(crossings[c])*rate/measured;
    check(std::abs(hz-expected)<expected*.04,"preview range/pitch differs");
    check(std::sqrt(energy[c]/measured)>.02,"preview is silent");
  }
  ok(riffine_preview_read(source.get(),block.data(),1,&count,error,sizeof(error)));check(!count,"EOF not stable");
  ok(riffine_preview_info_get(source.get(),&info,error,sizeof(error)));check(info.decoded_frames==rate,"read beyond selection");
}
void shortRange(const std::filesystem::path &path) {
  auto source=open(path,0,480,.25,0);float block[1024];uint32_t count=0;uint64_t frames=0;double energy=0;
  for(;;) {ok(riffine_preview_read(source.get(),block,512,&count,error,sizeof(error)));if(!count) break;
    frames+=count;for(unsigned i=0;i<count;++i) energy+=double(block[i])*block[i];}
  check(frames==1920 && energy>.001,"short preview duration/signal failed");
}
void callback() {
  riffine::PreviewQueue queue(7,2,3);float out[16],in[6]={1,2,3,4,5,6};
  allocations=0;trackAllocations=true;
  check(queue.consume(out,8)==0 && queue.underruns()==3 && !queue.copied(),"starvation counted as copied audio");
  check(queue.write(in,3) && queue.consume(out,8)==3,"callback audio copy failed");
  check(queue.copied()==3 && queue.underruns()==3,"EOF silence counted as underrun");
  check(queue.consume(out,8)==0 && queue.underruns()==3,"trailing callback changed underruns");
  trackAllocations=false;check(!allocations,"preview callback queue allocated");
}
}
int main() {
  try {
    Temp temp;auto mono=temp.path/std::filesystem::u8path(u8"guitär 音.wav");
    wav(mono,1,16,48000,3);
    for(double speed: {.25,.75,1.,2.}) signal(mono,1,48000,speed,speed==2 ? 12 : -12);
    shortRange(mono);callback();
    for(unsigned bits: {16u,24u,32u}) {
      auto stereo=temp.path/("stereo"+std::to_string(bits)+".wav");wav(stereo,2,bits,48000,3);signal(stereo,2,48000,.75,0);
    }
    for(unsigned rate: {8000u,192000u}) {
      auto path=temp.path/("rate"+std::to_string(rate)+".wav");wav(path,1,16,rate,3);signal(path,1,rate,1,0);
    }
    auto longSong=temp.path/"long.wav";wav(longSong,2,16,48000,1800,true);
    auto source=open(longSong,0,86400000,.75,3);float block[2048];uint32_t count=0;
    ok(riffine_preview_read(source.get(),block,1024,&count,error,sizeof(error)));
    riffine_preview_info info{};ok(riffine_preview_info_get(source.get(),&info,error,sizeof(error)));
    check(count==1024 && info.decoded_frames<=16384 && info.output_frames==115200000,"long source decoded whole selection before startup");
    auto longRender=temp.path/"long-render.wav";auto renderName=longRender.u8string();auto longName=longSong.u8string();
    largestAllocation=0;trackAllocations=true;
    auto renderResult=riffine_render(longName.c_str(),renderName.c_str(),0,86400000,2,0,error,sizeof(error));
    trackAllocations=false;ok(renderResult);
    check(largestAllocation<16*1024*1024,"offline render allocated duration-sized PCM storage");
    riffine_decoder *rendered=nullptr;ok(riffine_decoder_open_file(renderName.c_str(),&rendered,error,sizeof(error)));
    riffine_decoder_info renderedInfo{};ok(riffine_decoder_info_get(rendered,&renderedInfo,error,sizeof(error)));riffine_decoder_close(rendered);
    check(renderedInfo.frames==43200000 && renderedInfo.channels==2 && renderedInfo.source_bits==16,"long render dimensions differ");
    check(std::filesystem::remove(longRender),"cannot remove long rendered fixture");
    riffine_preview_source *bad=nullptr;auto name=mono.u8string();
    check(riffine_preview_open(name.c_str(),0,999999999,1,0,&bad,error,sizeof(error)) && !bad,"out of bounds accepted");
    riffine_preview_report report{};
    ok(riffine_preview(name.c_str(),0,4800,.5,0,1,&report,error,sizeof(error)));
    check(report.version==1 && report.copied_frames==9600 && report.output_frames==9600 && std::string(report.backend)=="Null","null device diagnostics differ");
    std::printf("preview source/queue/null-device checks passed; long-source first block decoded %llu of 86400000 frames\n",(unsigned long long)info.decoded_frames);
    return 0;
  } catch(const std::exception &e) {trackAllocations=false;std::fprintf(stderr,"%s\n",e.what());return 1;}
}
