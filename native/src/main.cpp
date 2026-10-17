#include "riffine_audio.h"
#include "riffine_control.h"
#include "riffine_output_backend.h"
#include "riffine_preview.h"
#include "riffine_decode.h"
#include <cstring>
#ifdef _WIN32
#include <io.h>
#include <fcntl.h>
#endif
#include <cstdio>
#include <stdexcept>
#include <string>
#include <vector>
#ifdef _WIN32
#include <windows.h>
#endif

int run(int argc, char **argv) {
  char error[1024]{};
  try {
    if(argc==2 && std::string(argv[1])=="version") {std::puts("riffine-audio ABI 1; Signalsmith Stretch + miniaudio");return 0;}
    int result=1;
    auto operation = argc > 1 ? std::string(argv[1]) : std::string();
    if(argc==2 && operation=="formats") {
#ifdef _WIN32
      std::puts(R"({"abi":1,"formats":["wav-pcm","flac","mp3","aac","alac"],"borrowedSources":true,"borrowedPreview":true,"borrowedRender":true,"scheduledControl":2,"scheduledCues":1})");
#elif defined(RIFFINE_ALAC)
      std::puts(R"({"abi":1,"formats":["wav-pcm","flac","mp3","alac"],"borrowedSources":true,"borrowedPreview":true,"borrowedRender":true,"scheduledControl":2,"scheduledCues":1})");
#else
      std::puts(R"({"abi":1,"formats":["wav-pcm","flac","mp3"],"borrowedSources":true,"borrowedPreview":true,"borrowedRender":true,"scheduledControl":2,"scheduledCues":1})");
#endif
      return 0;
    }
    if(argc==2 && operation=="device-info"){
      auto info=riffine::probeOutputBackend();
      std::printf(R"({"sampleRate":%u,"channels":%u,"bufferFrames":%u,"latencyFrames":%u})" "\n",info.sample_rate,info.channels,info.buffer_frames,info.latency_frames);return 0;
    }
    if(argc==3 && operation=="scheduled-control-handle"){
      std::string value=argv[2];if(value.empty()||value.find_first_not_of("0123456789")!=std::string::npos)throw std::runtime_error("invalid source handle");
      return riffine_scheduled_control(std::stoull(value));
    }
    if((argc==3 && (operation=="decoder-info" || operation=="decoder-info-handle")) || (argc==4 && (operation=="decoder-read" || operation=="decoder-read-handle"))) {
      riffine_decoder *raw=nullptr;
      if(operation=="decoder-info-handle" || operation=="decoder-read-handle"){
        std::string value=argv[2];
        if(value.empty()||value.find_first_not_of("0123456789")!=std::string::npos)throw std::runtime_error("invalid source handle");
        if(riffine_decoder_open_handle(std::stoull(value),&raw,error,sizeof(error)))throw std::runtime_error(error);
      } else if(riffine_decoder_open_file(argv[2],&raw,error,sizeof(error))) throw std::runtime_error(error);
      struct Owner {riffine_decoder *value;~Owner(){riffine_decoder_close(value);}} owner{raw};
      riffine_decoder_info info{};
      if(riffine_decoder_info_get(raw,&info,error,sizeof(error))) throw std::runtime_error(error);
      const char *encoding=info.encoding==1 ? "wav-pcm" : info.encoding==2 ? "flac" : info.encoding==3 ? "mp3" : info.encoding==4 ? "aac" : "alac";
      if(operation=="decoder-info" || operation=="decoder-info-handle") {
        std::printf(R"({"encoding":"%s","sampleRate":%u,"channels":%u,"bits":%u,"frames":%llu})" "\n",encoding,info.sample_rate,info.channels,info.source_bits,(unsigned long long)info.frames);return 0;
      }
      auto start=std::stoull(argv[3]);
      if(riffine_decoder_seek(raw,start,error,sizeof(error))) throw std::runtime_error(error);
#ifdef _WIN32
      if(_setmode(_fileno(stdout),_O_BINARY)==-1) throw std::runtime_error("cannot set binary PCM output");
#endif
      std::vector<float> pcm(8192*info.channels);std::vector<unsigned char> bytes(pcm.size()*4);
      for(;;) {
        uint32_t frames=0;
        if(riffine_decoder_read(raw,pcm.data(),8192,&frames,error,sizeof(error))) throw std::runtime_error(error);
        if(!frames)break;
        for(size_t i=0;i<size_t(frames)*info.channels;++i){uint32_t word;std::memcpy(&word,&pcm[i],4);for(unsigned b=0;b<4;++b)bytes[i*4+b]=static_cast<unsigned char>(word>>(8*b));}
        auto count=size_t(frames)*info.channels*4;
        if(std::fwrite(bytes.data(),1,count,stdout)!=count)throw std::runtime_error("cannot write decoded PCM");
      }
      if(std::fflush(stdout)!=0)throw std::runtime_error("cannot flush decoded PCM");return 0;
    }
    if(argc==8 && (operation=="render" || operation=="render-handle")) {
      if(operation=="render-handle"){
        std::string value=argv[2];if(value.empty()||value.find_first_not_of("0123456789")!=std::string::npos)throw std::runtime_error("invalid source handle");
        result=riffine_render_handle(std::stoull(value),argv[3],std::stoll(argv[4]),std::stoll(argv[5]),std::stod(argv[6]),std::stod(argv[7]),error,sizeof(error));
      }else result=riffine_render(argv[2],argv[3],std::stoll(argv[4]),std::stoll(argv[5]),std::stod(argv[6]),std::stod(argv[7]),error,sizeof(error));
    } else if(argc==8 && (operation=="preview" || operation=="preview-handle")) {
      std::string mode=argv[7];
      if(mode!="system" && mode!="null") throw std::runtime_error("preview mode must be system or null");
      riffine_preview_report report{};
      if(operation=="preview-handle"){
        std::string value=argv[2];if(value.empty()||value.find_first_not_of("0123456789")!=std::string::npos)throw std::runtime_error("invalid source handle");
        result=riffine_preview_handle(std::stoull(value),std::stoll(argv[3]),std::stoll(argv[4]),std::stod(argv[5]),std::stod(argv[6]),mode=="null",&report,error,sizeof(error));
      } else result=riffine_preview(argv[2],std::stoll(argv[3]),std::stoll(argv[4]),std::stod(argv[5]),std::stod(argv[6]),mode=="null",&report,error,sizeof(error));
      if(!result) std::printf(R"({"version":1,"channels":%u,"sampleRate":%u,"outputFrames":%llu,"copiedFrames":%llu,"underrunFrames":%llu,"decodedBeforeStart":%llu,"startupMilliseconds":%.3f,"backend":"%s","mode":"%s"})" "\n",
        report.channels,report.sample_rate,(unsigned long long)report.output_frames,(unsigned long long)report.copied_frames,
        (unsigned long long)report.underrun_frames,(unsigned long long)report.decoded_before_start,report.startup_milliseconds,report.backend,mode.c_str());
    } else if(argc==4 && std::string(argv[1])=="play") {
      if(std::string(argv[3])!="0" && std::string(argv[3])!="1") throw std::runtime_error("loop must be 0 or 1");
      result=riffine_play(argv[2],std::string(argv[3])=="1",error,sizeof(error));
    } else {throw std::runtime_error("usage: riffine-audio render IN OUT START_FRAME END_FRAME SPEED SEMITONES | preview IN START_FRAME END_FRAME SPEED SEMITONES system|null | play WAV LOOP | version");}
    if(result)std::fprintf(stderr,"%s\n",error);return result;
  } catch(const std::exception &e) {std::fprintf(stderr,"%s\n",e.what());return 1;}
}

#ifdef _WIN32
int wmain(int argc,wchar_t **argv) {
  std::vector<std::string> values;values.reserve(argc);
  for(int i=0;i<argc;++i){int n=WideCharToMultiByte(CP_UTF8,0,argv[i],-1,nullptr,0,nullptr,nullptr);std::string s(size_t(n),'\0');WideCharToMultiByte(CP_UTF8,0,argv[i],-1,s.data(),n,nullptr,nullptr);s.resize(size_t(n-1));values.push_back(s);}
  std::vector<char*> args;for(auto &v:values)args.push_back(v.data());return run(argc,args.data());
}
#else
int main(int argc,char **argv) {return run(argc,argv);}
#endif
