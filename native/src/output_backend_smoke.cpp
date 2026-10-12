#include "riffine_output_backend.h"
#include <array>
#include <chrono>
#include <cstdio>
#include <stdexcept>
#include <thread>
namespace {
using Clock=std::chrono::steady_clock;
void check(bool b,const char *message){if(!b)throw std::runtime_error(message);}
uint64_t observe(riffine::OutputBackend &device,riffine::PresentationQueue &queue){uint64_t presented=0;check(queue.observe(device.deviceFrame(),presented),"presentation observation failed");return presented;}
void suspend(riffine::OutputBackend &device,riffine::PresentationQueue &queue){
 check(queue.requestPause(),"pause request failed");auto until=Clock::now()+std::chrono::seconds(3);
 for(;;){auto presented=observe(device,queue);if(queue.pauseAcknowledged()&&presented==queue.copied())break;
  check(Clock::now()<until,"pause hardware tail did not present");std::this_thread::sleep_for(std::chrono::milliseconds(1));}
 device.stop();
}
}
int main(){try{
 auto info=riffine::probeOutputBackend();riffine::PresentationQueue queue(32768,info.channels,1024);
 auto device=riffine::openOutputBackend(queue,info.channels,info.sample_rate);
 std::array<float,8192*2> silence{};check(queue.write(silence.data(),8192),"cannot enqueue silence");
 uint64_t previous=0;
 for(unsigned run=0;run<3;++run){
  check(queue.start(),"queue start failed");device->start();auto until=Clock::now()+std::chrono::seconds(3);
  while(observe(*device,queue)<=previous){check(Clock::now()<until,"device clock did not advance");std::this_thread::sleep_for(std::chrono::milliseconds(1));}
  suspend(*device,queue);previous=queue.copied();check(previous>0,"silent PCM did not present");
 }
 device->resetClock();queue.reset();
 std::printf(R"({"backend":"CoreAudio-HAL","sampleRate":%u,"channels":%u,"bufferFrames":%u,"latencyFrames":%u,"presentedSilentFrames":%llu,"startSuspendCycles":3})" "\n",info.sample_rate,info.channels,info.buffer_frames,info.latency_frames,(unsigned long long)previous);
 return 0;
}catch(const std::exception &e){std::fprintf(stderr,"%s\n",e.what());return 1;}}
