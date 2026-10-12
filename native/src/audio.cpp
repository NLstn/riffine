#include "riffine_audio.h"
#include "riffine_preview.h"
#define NOMINMAX
#define MINIAUDIO_IMPLEMENTATION
#include "miniaudio.h"
#include <memory>
#include <algorithm>
#include <cmath>
#include <cstdio>
#include <filesystem>
#include <stdexcept>
#include <vector>

namespace {
void require(bool ok, const char *message) { if (!ok) throw std::runtime_error(message); }
void error_text(char *buffer, size_t size, const char *message) {
  if (buffer && size) std::snprintf(buffer, size, "%s", message);
}
struct Encoder {
  ma_encoder value{};
  FILE *file=nullptr;
  std::filesystem::path path;
  bool complete=false;
  static ma_result write(ma_encoder *encoder,const void *buffer,size_t bytes,size_t *written) {
    *written=std::fwrite(buffer,1,bytes,static_cast<FILE*>(encoder->pUserData));
    return *written==bytes ? MA_SUCCESS : MA_IO_ERROR;
  }
  static ma_result seek(ma_encoder *encoder,ma_int64 offset,ma_seek_origin origin) {
#ifdef _WIN32
    auto result=_fseeki64(static_cast<FILE*>(encoder->pUserData),offset,origin==ma_seek_origin_start ? SEEK_SET : SEEK_CUR);
#else
    auto result=fseeko(static_cast<FILE*>(encoder->pUserData),offset,origin==ma_seek_origin_start ? SEEK_SET : SEEK_CUR);
#endif
    return result==0 ? MA_SUCCESS : MA_IO_ERROR;
  }
  Encoder(const char *name, unsigned channels, unsigned rate):path(std::filesystem::u8path(name)) {
    auto config = ma_encoder_config_init(ma_encoding_format_wav, ma_format_s16, channels, rate);
#ifdef _WIN32
    file=_wfopen(path.c_str(),L"wbx");
#else
    file=std::fopen(name,"wbx");
#endif
    require(file!=nullptr,"cannot exclusively create output WAV");
    if(ma_encoder_init(write,seek,file,&config,&value)!=MA_SUCCESS){std::fclose(file);file=nullptr;std::error_code ec;std::filesystem::remove(path,ec);throw std::runtime_error("cannot initialize WAV encoder");}
  }
  ~Encoder() { if(file){ma_encoder_uninit(&value);std::fclose(file);} if(!complete){std::error_code ec;std::filesystem::remove(path,ec);} }
  void finish() {
    ma_encoder_uninit(&value);
    auto result=std::fclose(file);file=nullptr;
    require(result==0,"cannot finalize output WAV");
    complete=true;
  }
};
struct Engine {
  ma_engine value{};
  Engine() { require(ma_engine_init(nullptr, &value) == MA_SUCCESS, "cannot open an audio output device"); }
  ~Engine() { ma_engine_uninit(&value); }
};
struct Sound {
  ma_sound value{};
  Sound(ma_engine *engine, const char *path) {
#ifdef _WIN32
    require(ma_sound_init_from_file_w(engine, std::filesystem::u8path(path).c_str(), MA_SOUND_FLAG_STREAM, nullptr, nullptr, &value) == MA_SUCCESS, "cannot open playback file");
#else
    require(ma_sound_init_from_file(engine, path, MA_SOUND_FLAG_STREAM, nullptr, nullptr, &value) == MA_SUCCESS, "cannot open playback file");
#endif
  }
  ~Sound() { ma_sound_uninit(&value); }
};
}

static int render(const char *input,uint64_t handle,bool borrowed,const char *output,int64_t start,int64_t end,
 double speed,double semitones,char *error,size_t error_size) {
 if(error && error_size) error[0]=0;
 try {
  require((borrowed || input) && output,"source and output are required");
  require(!std::filesystem::exists(std::filesystem::u8path(output)),"output already exists");
  riffine_preview_source *raw=nullptr;char text[512];
  auto result=borrowed ? riffine_preview_open_handle(handle,start,end,speed,semitones,&raw,text,sizeof(text)) : riffine_preview_open(input,start,end,speed,semitones,&raw,text,sizeof(text));
  if(result)throw std::runtime_error(text);
  std::unique_ptr<riffine_preview_source,decltype(&riffine_preview_close)> owner(raw,riffine_preview_close);
  riffine_preview_info info{};
  if(riffine_preview_info_get(raw,&info,text,sizeof(text)))throw std::runtime_error(text);
  require(info.output_frames<=(UINT32_MAX-36)/(2*info.channels),"render exceeds PCM WAV size limit");
  Encoder encoder(output,info.channels,info.sample_rate);
  std::vector<float> block(4096*info.channels);
  std::vector<int16_t> pcm(block.size());uint64_t frames=0;
  for(;;){
   uint32_t count=0;
   if(riffine_preview_read(raw,block.data(),4096,&count,text,sizeof(text)))throw std::runtime_error(text);
   if(!count)break;
   for(size_t i=0;i<size_t(count)*info.channels;++i){
    require(std::isfinite(block[i]),"non-finite DSP output");
    pcm[i]=int16_t(std::lround(std::clamp(double(block[i]),-1.0,1.0)*32767));
   }
   ma_uint64 written=0;
   require(ma_encoder_write_pcm_frames(&encoder.value,pcm.data(),count,&written)==MA_SUCCESS && written==count,"cannot write rendered audio");
   frames+=count;
  }
  require(frames==info.output_frames,"incomplete rendered selection");
  encoder.finish();return 0;
 }catch(const std::exception &e){error_text(error,error_size,e.what());return 1;}
 catch(...){error_text(error,error_size,"unexpected native error");return 1;}
}
int riffine_render(const char *input,const char *output,int64_t start,int64_t end,double speed,double semitones,char *error,size_t error_size){
 return render(input,0,false,output,start,end,speed,semitones,error,error_size);
}
int riffine_render_handle(uint64_t handle,const char *output,int64_t start,int64_t end,double speed,double semitones,char *error,size_t error_size){
 return render(nullptr,handle,true,output,start,end,speed,semitones,error,error_size);
}

int riffine_play(const char *input, int loop, char *error, size_t error_size) {
  try {
    require(input,"input path required");Engine engine;Sound sound(&engine.value,input);
    ma_sound_set_looping(&sound.value,loop ? MA_TRUE : MA_FALSE);
    require(ma_sound_start(&sound.value)==MA_SUCCESS,"cannot start playback");
    while(!ma_sound_at_end(&sound.value)) ma_sleep(20);
    return 0;
  } catch(const std::exception &e) {error_text(error,error_size,e.what());return 1;}
  catch(...) {error_text(error,error_size,"unexpected native error");return 1;}
}
