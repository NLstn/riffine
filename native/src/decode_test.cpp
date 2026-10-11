#include "riffine_decode.h"
#include <algorithm>
#include <chrono>
#include <cmath>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <stdexcept>
#include <string>
#include <vector>
namespace fs=std::filesystem;
void check(bool ok,const char *message){if(!ok)throw std::runtime_error(message);}
int main(){
 auto root=fs::temp_directory_path()/fs::u8path("riffine-decode-音楽-"+std::to_string(std::chrono::steady_clock::now().time_since_epoch().count()));
 try {
  fs::create_directories(root);
  for(const char *extension:{"flac","mp3"}){
   auto input=root/fs::u8path(std::string("ギター.")+extension);
   fs::copy_file(fs::path(RIFFINE_DECODE_FIXTURES)/(std::string("generated-stereo.")+extension),input);
   auto text=input.u8string();char error[512];riffine_decoder *s=nullptr;
   check(!riffine_decoder_open_file(text.c_str(),&s,error,sizeof(error)),error);
   struct Owner{riffine_decoder *s;~Owner(){riffine_decoder_close(s);}}owner{s};
   riffine_decoder_info info{};check(!riffine_decoder_info_get(s,&info,error,sizeof(error)),error);
   check(info.channels==2&&info.sample_rate==48000&&info.frames==48000,"unexpected fixture coordinates");
   check(info.encoding==(std::string(extension)=="flac"?2:3)&&info.source_bits==(info.encoding==2?16:0),"incorrect codec/precision metadata");
   std::vector<float> all(size_t(info.frames)*2),block(8192*2);uint64_t offset=0;
   while(offset<info.frames){uint32_t n=0;check(!riffine_decoder_read(s,block.data(),8192,&n,error,sizeof(error))&&n>0,error);std::copy_n(block.data(),n*2,all.data()+offset*2);offset+=n;}
   uint32_t n=1;check(!riffine_decoder_read(s,block.data(),8192,&n,error,sizeof(error))&&n==0,"EOF did not stop");
   for(uint64_t start:{uint64_t(0),uint64_t(1),uint64_t(1151),uint64_t(1152),uint64_t(12345),info.frames-17,info.frames}){
    check(!riffine_decoder_seek(s,start,error,sizeof(error)),error);
    check(riffine_decoder_seek(s,info.frames+1,error,sizeof(error))!=0,"accepted invalid seek");
    check(riffine_decoder_read(s,block.data(),0,&n,error,sizeof(error))!=0,"accepted empty read");
    check(!riffine_decoder_read(s,block.data(),8192,&n,error,sizeof(error)),error);
    check(n==std::min<uint64_t>(8192,info.frames-start),"seek produced wrong frame count");
    check(std::equal(block.begin(),block.begin()+size_t(n)*2,all.begin()+size_t(start)*2),"seek differs from sequential decoded coordinates");
   }
   for(unsigned c=0;c<2;++c){unsigned crossings=0;double squares=0;for(unsigned j=2400;j<45600;++j){auto v=all[j*2+c];check(std::isfinite(v),"non-finite PCM");squares+=double(v)*v;if(j>2400&&all[(j-1)*2+c]<=0&&v>0)crossings++;}double hz=crossings*48000./43200;check(std::abs(hz-(c?660:440))<3,"fixture frequency/channel mismatch");check(std::sqrt(squares/43200)>.1,"fixture energy mismatch");}
  }
  auto malformed=root/"bad.mp3";std::ofstream(malformed)<<"not audio";riffine_decoder *bad=nullptr;char error[512];check(riffine_decoder_open_file(malformed.u8string().c_str(),&bad,error,sizeof(error))!=0&&bad==nullptr,"malformed audio accepted");
  fs::remove_all(root);return 0;
 }catch(const std::exception &e){std::cerr<<e.what()<<"\n";fs::remove_all(root);return 1;}
}
