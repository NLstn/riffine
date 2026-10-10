#include "riffine_audio.h"
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
    if(argc==8 && std::string(argv[1])=="render") {
      result=riffine_render(argv[2],argv[3],std::stoll(argv[4]),std::stoll(argv[5]),std::stod(argv[6]),std::stod(argv[7]),error,sizeof(error));
    } else if(argc==4 && std::string(argv[1])=="play") {
      if(std::string(argv[3])!="0" && std::string(argv[3])!="1") throw std::runtime_error("loop must be 0 or 1");
      result=riffine_play(argv[2],std::string(argv[3])=="1",error,sizeof(error));
    } else {throw std::runtime_error("usage: riffine-audio render IN OUT START_FRAME END_FRAME SPEED SEMITONES | play WAV LOOP | version");}
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
