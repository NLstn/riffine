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
#ifdef _WIN32
#define NOMINMAX
#include <windows.h>
#endif
namespace fs=std::filesystem;
void check(bool ok,const char *message){if(!ok)throw std::runtime_error(message);}
int main(){
 auto root=fs::temp_directory_path()/fs::u8path("riffine-decode-音楽-"+std::to_string(std::chrono::steady_clock::now().time_since_epoch().count()));
 try {
  fs::create_directories(root);
  std::vector<const char *> extensions={"flac","mp3"};
#ifdef _WIN32
  extensions.push_back("m4a");extensions.push_back("alac.m4a");
#endif
  for(const char *extension:extensions){
   auto input=root/fs::u8path(std::string("ギター.")+extension);
   fs::copy_file(fs::path(RIFFINE_DECODE_FIXTURES)/(std::string("generated-stereo.")+extension),input);
   auto text=input.u8string();char error[512];riffine_decoder *s=nullptr;
   check(!riffine_decoder_open_file(text.c_str(),&s,error,sizeof(error)),error);
   struct Owner{riffine_decoder *s;~Owner(){riffine_decoder_close(s);}}owner{s};
   riffine_decoder_info info{};check(!riffine_decoder_info_get(s,&info,error,sizeof(error)),error);
   auto name=std::string(extension);bool m4a=name=="m4a"||name=="alac.m4a";
   check(info.channels==2&&info.sample_rate==48000&&(m4a?(info.frames>=48000&&info.frames<=50176):info.frames==48000),"unexpected fixture coordinates");
   auto encoding=name=="flac"?2:name=="mp3"?3:name=="m4a"?4:5;
   check(info.encoding==encoding&&info.source_bits==(info.encoding==2?16:0),"incorrect codec/precision metadata");
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
#ifdef _WIN32
   // Borrowed-handle decoding must match path decoding and preserve ownership
   // and the caller's cursor, including after the source is renamed.
   // The legacy path decoder's CRT file intentionally does not share deletes.
   riffine_decoder_close(owner.s);owner.s=nullptr;
   auto handle=CreateFileW(input.c_str(),GENERIC_READ,FILE_SHARE_READ|FILE_SHARE_DELETE,nullptr,OPEN_EXISTING,FILE_ATTRIBUTE_NORMAL,nullptr);
   check(handle!=INVALID_HANDLE_VALUE,"cannot open borrowed fixture");
   struct HandleOwner{HANDLE h;~HandleOwner(){CloseHandle(h);}}handleOwner{handle};
   LARGE_INTEGER at{};at.QuadPart=19;check(SetFilePointerEx(handle,at,nullptr,FILE_BEGIN),"cannot set caller cursor");
   auto renamed=input;renamed+=".renamed";fs::rename(input,renamed);
   riffine_decoder *borrowed=nullptr;
   check(!riffine_decoder_open_handle(uint64_t(uintptr_t(handle)),&borrowed,error,sizeof(error)),error);
   Owner borrowedOwner{borrowed};riffine_decoder_info borrowedInfo{};
   check(!riffine_decoder_info_get(borrowed,&borrowedInfo,error,sizeof(error))&&borrowedInfo.frames==info.frames&&borrowedInfo.encoding==info.encoding,"borrowed metadata mismatch");
   check(!riffine_decoder_seek(borrowed,12345,error,sizeof(error)),error);
   check(!riffine_decoder_read(borrowed,block.data(),8192,&n,error,sizeof(error))&&n==8192,error);
   check(std::equal(block.begin(),block.begin()+n*2,all.begin()+12345*2),"borrowed seek PCM mismatch");
   at.QuadPart=0;LARGE_INTEGER cursor{};
   check(SetFilePointerEx(handle,at,&cursor,FILE_CURRENT)&&cursor.QuadPart==19,"borrowed decoder changed caller cursor");
#endif
  }
  auto malformed=root/"bad.mp3";std::ofstream(malformed)<<"not audio";riffine_decoder *bad=nullptr;char error[512];check(riffine_decoder_open_file(malformed.u8string().c_str(),&bad,error,sizeof(error))!=0&&bad==nullptr,"malformed audio accepted");
#ifdef _WIN32
  auto badM4A=root/"bad.m4a";{std::ofstream file(badM4A,std::ios::binary);file.write("\0\0\0\x18" "ftypM4A ",12);}
  check(riffine_decoder_open_file(badM4A.u8string().c_str(),&bad,error,sizeof(error))!=0&&bad==nullptr,"malformed M4A accepted");
#endif
  fs::remove_all(root);return 0;
 }catch(const std::exception &e){std::cerr<<e.what()<<"\n";fs::remove_all(root);return 1;}
}
