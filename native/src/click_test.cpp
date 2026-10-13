#include "riffine_click.h"
#include <algorithm>
#include <cmath>
#include <cstdlib>
#include <iostream>
#include <limits>
#include <stdexcept>
#include <vector>

static void require(bool pass,const char *message){if(!pass)throw std::runtime_error(message);}
int main(){try{
 constexpr uint32_t rate=48000,frames=2048,channels=2;
 const riffine_click_event events[]={{100,1,0},{960,0,0}};
 char error[256]={};
 std::vector<float> whole(frames*channels),chunked(frames*channels);
 require(!riffine_click_mix(1,channels,rate,0,frames,1,events,2,whole.data(),error,sizeof(error)),error);
 for(uint32_t start=0;start<frames;start+=127){const auto count=std::min<uint32_t>(127,frames-start);require(!riffine_click_mix(1,channels,rate,start,count,1,events,2,chunked.data()+start*channels,error,sizeof(error)),error);}
 require(whole==chunked,"click phase depends on block partition");
 double energy=0;float peak=0;
 for(uint32_t frame=0;frame<frames;++frame){require(whole[frame*2]==whole[frame*2+1],"click stereo channels differ");energy+=whole[frame*2]*whole[frame*2];peak=std::max(peak,std::abs(whole[frame*2]));if(frame<100 || (frame>=580 && frame<960) || frame>=1440)require(whole[frame*2]==0,"click escaped its 10 ms tail");}
 require(energy>1 && peak>.15 && peak<=.25,"click envelope has wrong energy or gain");
 auto crossings=[&](uint32_t start,uint32_t end){int count=0;for(uint32_t frame=start+1;frame<end;++frame)if(whole[(frame-1)*2]<=0 && whole[frame*2]>0)++count;return count;};
 require(std::abs(crossings(100,580)-15)<=1 && std::abs(crossings(960,1440)-10)<=1,"click carrier frequency is wrong");
 std::vector<float> half(frames*2);
 require(!riffine_click_mix(1,2,rate,0,frames,.5f,events,2,half.data(),error,sizeof(error)),error);
 for(size_t i=0;i<half.size();++i)require(std::abs(half[i]-whole[i]*.5f)<1e-7f,"click level is not linear");
 std::vector<float> source(frames*2,.98f),saved=source;
 require(riffine_click_mix(1,2,rate,0,frames,1,events,257,source.data(),error,sizeof(error))!=0 && source==saved,"invalid event count mutated output");
 riffine_click_event unordered[]={{960,0,0},{100,1,0}};
 require(riffine_click_mix(1,2,rate,0,frames,1,unordered,2,source.data(),error,sizeof(error))!=0 && source==saved,"unordered events mutated output");
 require(!riffine_click_mix(1,2,rate,0,frames,0,events,2,source.data(),error,sizeof(error)) && source==saved,"disabled click altered audio");
 require(!riffine_click_mix(1,2,rate,0,frames,1,events,2,source.data(),error,sizeof(error)),error);
 for(float value:source)require(value<=1 && value>=-1,"click mixing exceeded PCM range");
 // A click close to the phrase end uses a shorter complete envelope, rather
 // than truncating a full-amplitude tail at the loop/rest boundary.
 const riffine_click_event edge[]={{180,0,0}};std::vector<float> shortWhole(200),shortParts(200);
 require(!riffine_click_mix_window(1,1,rate,0,200,1,edge,1,shortWhole.data(),200,error,sizeof(error)),error);
 for(uint32_t start=0;start<200;start+=7)require(!riffine_click_mix_window(1,1,rate,start,std::min<uint32_t>(7,200-start),1,edge,1,shortParts.data()+start,200,error,sizeof(error)),error);
 require(shortWhole==shortParts,"bounded click tail depends on block size");
 require(std::abs(shortWhole.back())<.002,"bounded click tail ends abruptly");
 if(std::getenv("RIFFINE_CLICK_BOUNDARY_FIXTURE")){
  std::cout.precision(9);std::cout<<"[";for(size_t i=0;i<shortWhole.size();++i){if(i)std::cout<<",";std::cout<<shortWhole[i];}std::cout<<"]\n";
 }else if(std::getenv("RIFFINE_CLICK_FIXTURE")){
  std::cout.precision(9);std::cout<<"[";for(size_t i=0;i<whole.size();++i){if(i)std::cout<<",";std::cout<<whole[i];}std::cout<<"]\n";
 }else std::cout<<"shared click synthesis passed\n";
 return 0;
}catch(const std::exception &error){std::cerr<<error.what()<<"\n";return 1;}}
