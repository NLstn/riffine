#include "riffine_scheduled.h"
#include "riffine_audio.h"
#include "riffine_decode.h"
#include "riffine_preview.h"
#include "encoded_file.h"
#include <algorithm>
#include <chrono>
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <new>
#include <filesystem>
#include <iostream>
#include <memory>
#include <stdexcept>
#include <string>
#include <vector>
static bool trackAllocations=false;
static unsigned allocations=0;
void *operator new(size_t size){if(trackAllocations)++allocations;if(void *p=std::malloc(size?size:1))return p;throw std::bad_alloc();}
void *operator new[](size_t size){return ::operator new(size);}
void operator delete(void *p) noexcept{std::free(p);}
void operator delete[](void *p) noexcept{std::free(p);}
void operator delete(void *p,size_t) noexcept{std::free(p);}
void operator delete[](void *p,size_t) noexcept{std::free(p);}
namespace fs=std::filesystem;
namespace {
char error[512];
void check(bool ok,const char *message){if(!ok)throw std::runtime_error(message);}
void ok(int result){if(result)throw std::runtime_error(error);}
void read(riffine_scheduled_worker *worker,float *output,uint32_t capacity,riffine_scheduled_block *report){
 allocations=0;trackAllocations=true;
 auto result=riffine_scheduled_read(worker,output,capacity,report,error,sizeof(error));
 trackAllocations=false;ok(result);check(!allocations,"scheduled read allocated C++ memory");
}
using Worker=std::unique_ptr<riffine_scheduled_worker,decltype(&riffine_scheduled_close)>;
void little(FILE *file,uint64_t n,unsigned bytes){for(unsigned i=0;i<bytes;++i)check(std::fputc(int((n>>(8*i))&255),file)!=EOF,"fixture write failed");}
void wav(const fs::path &path){
#ifdef _WIN32
 FILE *raw=_wfopen(path.c_str(),L"wbx");
#else
 FILE *raw=std::fopen(path.c_str(),"wbx");
#endif
 check(raw,"exclusive fixture creation failed");std::unique_ptr<FILE,decltype(&std::fclose)> owner(raw,std::fclose);
 check(std::fwrite("RIFF",1,4,raw)==4,"fixture header");little(raw,192036,4);
 check(std::fwrite("WAVEfmt ",1,8,raw)==8,"fixture header");little(raw,16,4);little(raw,1,2);little(raw,2,2);little(raw,48000,4);little(raw,192000,4);little(raw,4,2);little(raw,16,2);
 check(std::fwrite("data",1,4,raw)==4,"fixture header");little(raw,192000,4);
 for(unsigned i=0;i<48000;++i)for(unsigned c=0;c<2;++c)little(raw,uint64_t(int64_t(std::llround(.2*32767*std::sin(6.283185307179586*(c?660:440)*i/48000)))),2);
 check(std::fflush(raw)==0,"fixture flush");
}
riffine_schedule schedule(uint64_t generation,uint64_t sequence,uint64_t start,uint64_t end,uint64_t output,double speed,double pitch){
 auto count=std::max<uint64_t>(1,uint64_t(std::llround(double(end-start)/speed)));
 return {generation,sequence,start,end,output,output+count,std::min<uint64_t>(240,(count+1)/2),speed,pitch};
}
std::vector<float> reference(const fs::path &path,const riffine_schedule &s,uint64_t handle=UINT64_MAX){
 riffine_preview_source *raw=nullptr;auto name=path.u8string();if(handle==UINT64_MAX)ok(riffine_preview_open(name.c_str(),int64_t(s.start_frame),int64_t(s.end_frame),s.speed,s.semitones,&raw,error,sizeof(error)));
 else ok(riffine_preview_open_handle(handle,int64_t(s.start_frame),int64_t(s.end_frame),s.speed,s.semitones,&raw,error,sizeof(error)));
 std::unique_ptr<riffine_preview_source,decltype(&riffine_preview_close)> owner(raw,riffine_preview_close);
 std::vector<float> result,block(8192*2);uint32_t count;
 do {ok(riffine_preview_read(raw,block.data(),511,&count,error,sizeof(error)));result.insert(result.end(),block.begin(),block.begin()+size_t(count)*2);}while(count);
 return result;
}
struct InputResource {
#ifdef _WIN32
 HANDLE handle=INVALID_HANDLE_VALUE;
 explicit InputResource(const fs::path &path){handle=CreateFileW(path.c_str(),GENERIC_READ,FILE_SHARE_READ|FILE_SHARE_WRITE|FILE_SHARE_DELETE,nullptr,OPEN_EXISTING,FILE_ATTRIBUTE_NORMAL,nullptr);check(handle!=INVALID_HANDLE_VALUE,"cannot open caller resource");LARGE_INTEGER position{};position.QuadPart=17;check(SetFilePointerEx(handle,position,nullptr,FILE_BEGIN)!=0,"caller seek failed");}
 ~InputResource(){CloseHandle(handle);}
 uint64_t token() const{return uint64_t(reinterpret_cast<uintptr_t>(handle));}
 uint64_t position() const{LARGE_INTEGER zero{},position{};check(SetFilePointerEx(handle,zero,&position,FILE_CURRENT)!=0,"caller tell failed");return uint64_t(position.QuadPart);}
#else
 int handle=-1;
 explicit InputResource(const fs::path &path){handle=::open(path.c_str(),O_RDONLY);check(handle>=0,"cannot open caller resource");check(lseek(handle,17,SEEK_SET)==17,"caller seek failed");}
 ~InputResource(){::close(handle);}
 uint64_t token() const{return uint64_t(handle);}
 uint64_t position() const{auto position=lseek(handle,0,SEEK_CUR);check(position>=0,"caller tell failed");return uint64_t(position);}
#endif
};
void rendered(const fs::path &path,uint64_t handle,const riffine_schedule &s){
 auto expected=reference(path,s,handle);
 auto output=path;output+=".render.wav";auto name=output.u8string();
 ok(riffine_render_handle(handle,name.c_str(),int64_t(s.start_frame),int64_t(s.end_frame),s.speed,s.semitones,error,sizeof(error)));
 check(riffine_render_handle(handle,name.c_str(),int64_t(s.start_frame),int64_t(s.end_frame),s.speed,s.semitones,error,sizeof(error))!=0,"render replaced existing output");
 riffine_decoder *decoder=nullptr;ok(riffine_decoder_open_file(name.c_str(),&decoder,error,sizeof(error)));
 std::unique_ptr<riffine_decoder,decltype(&riffine_decoder_close)> owner(decoder,riffine_decoder_close);
 riffine_decoder_info info{};ok(riffine_decoder_info_get(decoder,&info,error,sizeof(error)));
 check(info.frames==s.output_end-s.output_start && info.channels==2 && info.source_bits==16,"render dimensions differ");
 std::vector<float> block(8192*2);uint64_t offset=0;uint32_t count=0;
 do{
  ok(riffine_decoder_read(decoder,block.data(),8192,&count,error,sizeof(error)));
  for(size_t i=0;i<size_t(count)*2;++i){
   auto q=std::lround(std::clamp(double(expected[size_t(offset)*2+i]),-1.0,1.0)*32767);
   check(block[i]==float(q)/32768,"render differs from quantized shared kernel");
  }
  offset+=count;
 }while(count);
 check(offset==info.frames,"render ended early");owner.reset();
 check(fs::remove(output),"cannot remove rendered fixture");
}
void exercise(const fs::path &path,bool borrowed=false){
 InputResource resource(path);
 riffine_scheduled_worker *raw=nullptr;auto name=path.u8string();
 if(borrowed)ok(riffine_scheduled_open_handle(resource.token(),&raw,error,sizeof(error)));else ok(riffine_scheduled_open(name.c_str(),&raw,error,sizeof(error)));
 Worker worker(raw,riffine_scheduled_close);
 riffine_scheduled_info info{};ok(riffine_scheduled_info_get(raw,&info,error,sizeof(error)));check(info.channels==2&&info.sample_rate==48000&&info.source_frames==48000&&!info.pending&&!info.generation,"incorrect source metadata");
 auto first=schedule(1,1,12000,36000,0,.75,3),second=schedule(1,2,1151,10001,first.output_end,2,-12);
 rendered(path,resource.token(),first);rendered(path,resource.token(),second);check(resource.position()==17,"render changed caller cursor");
 check(riffine_scheduled_submit(raw,&first,error,sizeof(error))!=0,"accepted schedule before reset");
 ok(riffine_scheduled_reset(raw,1,error,sizeof(error)));ok(riffine_scheduled_submit(raw,&first,error,sizeof(error)));ok(riffine_scheduled_submit(raw,&second,error,sizeof(error)));
 auto third=schedule(1,3,1,2,second.output_end,2,12);
 check(riffine_scheduled_submit(raw,&third,error,sizeof(error))!=0,"accepted third pending selection");
 check(riffine_scheduled_reset(raw,1,error,sizeof(error))!=0,"accepted stale reset");
 std::vector<float> block(8192*2);riffine_scheduled_block report{};
 check(riffine_scheduled_read(raw,block.data(),8193,&report,error,sizeof(error))!=0&&!report.frames,"accepted oversized read");
 for(const auto &s:{first,second}){
  auto expected=reference(path,s);check(reference(path,s,resource.token())==expected,"borrowed preview differs from path kernel");check(resource.position()==17,"borrowed preview changed caller cursor");std::vector<float> actual;uint64_t output=s.output_start;
  do {
   read(raw,block.data(),113,&report);
   check(report.generation==s.generation&&report.sequence==s.sequence&&report.output_start==output&&report.frames>0&&report.frames<=113,"incorrect schedule block identity/coordinates");
   for(unsigned i=0;i<report.frames*2;++i)check(std::isfinite(block[i]),"non-finite scheduled PCM");
   actual.insert(actual.end(),block.begin(),block.begin()+size_t(report.frames)*2);output+=report.frames;
  }while(!report.generated_complete);
  check(output==s.output_end&&actual==expected,"scheduled PCM differs from bounded preview kernel");
 }
 read(raw,block.data(),128,&report);check(!report.frames&&!report.generated_complete,"unscheduled audio repeated");
 ok(riffine_scheduled_info_get(raw,&info,error,sizeof(error)));check(!info.pending&&info.last_sequence==2,"incorrect empty-worker state");
 for(unsigned i=0;i<100;++i){
  ok(riffine_scheduled_reset(raw,2+i,error,sizeof(error)));
  auto tiny=schedule(2+i,3+i,1,2,0,2,12);
  auto invalid=tiny;invalid.output_end++;check(riffine_scheduled_submit(raw,&invalid,error,sizeof(error))!=0,"accepted wrong rounded output length");
  invalid=tiny;invalid.edge_fade_frames=2;check(riffine_scheduled_submit(raw,&invalid,error,sizeof(error))!=0,"accepted excessive fade");
  ok(riffine_scheduled_submit(raw,&tiny,error,sizeof(error)));
  check(riffine_scheduled_submit(raw,&tiny,error,sizeof(error))!=0,"accepted duplicate sequence");
  read(raw,block.data(),128,&report);check(report.frames==1&&report.generated_complete&&report.output_start==0,"odd one-frame selection failed");
 }
 for(unsigned i=0;i<4;++i){
  const double speeds[]={.25,.75,1,2},pitches[]={-12,3,0,12};
  ok(riffine_scheduled_reset(raw,102+i,error,sizeof(error)));
  auto profile=schedule(102+i,103+i,12000,36000,0,speeds[i],pitches[i]);
  ok(riffine_scheduled_submit(raw,&profile,error,sizeof(error)));
  auto expected=reference(path,profile);uint64_t offset=0;
  do {
   read(raw,block.data(),8192,&report);
   check(report.output_start==offset&&report.sequence==profile.sequence&&report.generation==profile.generation,"profile block coordinates changed");
   check(std::equal(block.begin(),block.begin()+size_t(report.frames)*2,expected.begin()+size_t(offset)*2),"profile PCM differs from reference");
   offset+=report.frames;
  }while(!report.generated_complete);
  check(offset==profile.output_end,"profile output count differs");
 }
 // Reset while only part of a generated selection has been read; no old output
 // or schedule can escape into the replacement generation.
 ok(riffine_scheduled_reset(raw,106,error,sizeof(error)));first=schedule(106,107,12000,36000,0,.25,-12);ok(riffine_scheduled_submit(raw,&first,error,sizeof(error)));read(raw,block.data(),7,&report);
 ok(riffine_scheduled_reset(raw,107,error,sizeof(error)));third=schedule(107,108,12000,12001,0,1,0);ok(riffine_scheduled_submit(raw,&third,error,sizeof(error)));read(raw,block.data(),128,&report);check(report.generation==107&&report.sequence==108&&report.frames==1&&report.generated_complete,"reset leaked old-generation PCM");
 worker.reset();check(resource.position()==17,"worker closed caller handle or changed its cursor");
}
}
int main(){
 auto root=fs::temp_directory_path()/fs::u8path("riffine-scheduled-音楽-"+std::to_string(std::chrono::steady_clock::now().time_since_epoch().count()));
 try{
  check(fs::create_directory(root),"cannot create fixture directory");auto path=root/fs::u8path("ギター.wav");wav(path);exercise(path);exercise(path,true);
  for(const char *extension:{"flac","mp3"}){path=root/fs::u8path(std::string("ギター.")+extension);fs::copy_file(fs::path(RIFFINE_DECODE_FIXTURES)/(std::string("generated-stereo.")+extension),path);exercise(path);exercise(path,true);}
  fs::remove_all(root);std::cout<<"bounded WAV/FLAC/MP3 scheduled worker: 2 cursors, exact kernel parity, 100 resets; no device/presentation evidence\n";return 0;
 }catch(const std::exception &e){std::cerr<<e.what()<<"\n";fs::remove_all(root);return 1;}
}
