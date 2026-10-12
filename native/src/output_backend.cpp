#include "riffine_output_backend.h"
#include "riffine_device_clock.h"
#include <array>
#include <cmath>
#include <cstring>
#include <stdexcept>
#include <thread>
#include <chrono>
#ifdef __APPLE__
#include <CoreAudio/CoreAudio.h>
namespace riffine {namespace {
void check(bool ok,const char *message){if(!ok)throw std::runtime_error(message);}
template<class T>T property(AudioObjectID object,AudioObjectPropertySelector selector,AudioObjectPropertyScope scope=kAudioObjectPropertyScopeGlobal){
 AudioObjectPropertyAddress address{selector,scope,kAudioObjectPropertyElementMain};T value{};UInt32 size=sizeof(value);
 check(AudioObjectGetPropertyData(object,&address,0,nullptr,&size,&value)==noErr&&size==sizeof(value),"cannot read output device property");return value;
}
struct Configuration {AudioDeviceID device;AudioStreamID stream;OutputBackendInfo info;bool planar;};
Configuration configuration(){
 auto device=property<AudioDeviceID>(kAudioObjectSystemObject,kAudioHardwarePropertyDefaultOutputDevice);
 check(device!=kAudioObjectUnknown,"no default output device");
 AudioObjectPropertyAddress address{kAudioDevicePropertyStreams,kAudioDevicePropertyScopeOutput,kAudioObjectPropertyElementMain};UInt32 size=0;
 check(AudioObjectGetPropertyDataSize(device,&address,0,nullptr,&size)==noErr&&size==sizeof(AudioStreamID),"output device requires one stream");
 AudioStreamID stream=0;check(AudioObjectGetPropertyData(device,&address,0,nullptr,&size,&stream)==noErr,"cannot query output stream");
 auto format=property<AudioStreamBasicDescription>(stream,kAudioStreamPropertyVirtualFormat);
 auto channels=format.mChannelsPerFrame;auto planar=(format.mFormatFlags&kAudioFormatFlagIsNonInterleaved)!=0;
 check(std::isfinite(format.mSampleRate)&&format.mSampleRate>=8000&&format.mSampleRate<=192000&&std::floor(format.mSampleRate)==format.mSampleRate,"unsupported output sample rate");
 check(format.mFormatID==kAudioFormatLinearPCM&&channels>=1&&channels<=2&&format.mBitsPerChannel==32&&
  (format.mFormatFlags&kAudioFormatFlagIsFloat)&&(format.mFormatFlags&kAudioFormatFlagIsPacked)&&!(format.mFormatFlags&kAudioFormatFlagIsBigEndian)&&
  format.mBytesPerFrame==(planar?4:4*channels)&&format.mFramesPerPacket==1,"output device requires native float32 mono/stereo");
 auto period=property<UInt32>(device,kAudioDevicePropertyBufferFrameSize);
 auto deviceLatency=property<UInt32>(device,kAudioDevicePropertyLatency,kAudioDevicePropertyScopeOutput);
 auto streamLatency=property<UInt32>(stream,kAudioStreamPropertyLatency);
 check(period>0&&period<=8192&&uint64_t(deviceLatency)+streamLatency<=192000,"unsupported output buffer or latency");
 return {device,stream,{uint32_t(format.mSampleRate),channels,period,deviceLatency+streamLatency},planar};
}
class CoreAudioOutput final:public OutputBackend {
 struct Listener {AudioObjectID object;AudioObjectPropertyAddress address;};
 Configuration config_;
 PresentationQueue &queue_;
 const uint32_t source_channels_;
 DeviceClockMap clock_;
 AudioDeviceIOProcID proc_=nullptr;
 std::atomic<uint32_t> running_{0},lost_{0},callbacks_{0},notifications_{0};
 std::array<float,8192*2> scratch_{};
 std::array<Listener,7> listeners_{};unsigned registered_=0;
 static OSStatus changed(AudioObjectID,UInt32,const AudioObjectPropertyAddress *,void *context) noexcept{
  auto &self=*static_cast<CoreAudioOutput*>(context);self.notifications_.fetch_add(1,std::memory_order_acq_rel);
  self.lost_.store(1,std::memory_order_release);self.notifications_.fetch_sub(1,std::memory_order_release);return noErr;
 }
 static bool sample(const AudioTimeStamp *stamp,bool ceiling,int64_t &value) noexcept{
  if(!stamp||!(stamp->mFlags&kAudioTimeStampSampleTimeValid)||!std::isfinite(stamp->mSampleTime)||std::abs(stamp->mSampleTime)>=double(uint64_t(1)<<53))return false;
  value=int64_t(ceiling?std::ceil(stamp->mSampleTime):std::floor(stamp->mSampleTime));return true;
 }
 static OSStatus callback(AudioObjectID,const AudioTimeStamp *,const AudioBufferList *,const AudioTimeStamp *,AudioBufferList *output,const AudioTimeStamp *time,void *context) noexcept{
  auto &self=*static_cast<CoreAudioOutput*>(context);self.callbacks_.fetch_add(1,std::memory_order_acq_rel);
  struct Exit {CoreAudioOutput &s;~Exit(){s.callbacks_.fetch_sub(1,std::memory_order_release);}} exit{self};
  if(!output){self.lost_.store(1,std::memory_order_release);return noErr;}
  // CoreAudio supplies zeroed output buffers. Invalid shapes return that silence
  // without traversing an unbounded/unsupported driver buffer.
  if(!self.running_.load(std::memory_order_acquire))return noErr;
  auto fail=[&](){self.lost_.store(1,std::memory_order_release);self.queue_.failClosed();};
  if(self.lost_.load(std::memory_order_acquire)){fail();return noErr;}
  auto channels=self.config_.info.channels;auto buffers=self.config_.planar?channels:1;
  if(output->mNumberBuffers!=buffers){fail();return noErr;}
  auto perBuffer=self.config_.planar?1:channels;uint32_t frames=0;
  for(UInt32 i=0;i<buffers;++i){const auto &b=output->mBuffers[i];
   if(!b.mData||b.mNumberChannels!=perBuffer||b.mDataByteSize%(4*perBuffer)){fail();return noErr;}
   auto n=b.mDataByteSize/(4*perBuffer);if(!n||n>8192||(i&&n!=frames)){fail();return noErr;}frames=n;
  }
  int64_t hardware=0;uint64_t position=0;
  if(!sample(time,true,hardware)||!self.clock_.callbackFrame(hardware,frames,position)){fail();return noErr;}
  self.queue_.consume(self.scratch_.data(),frames,position);
  if(self.queue_.error()){fail();return noErr;}
  for(uint32_t i=0;i<frames;++i)for(uint32_t c=0;c<channels;++c){
   auto value=self.scratch_[size_t(i)*self.source_channels_+(self.source_channels_==1?0:c)];
   if(self.config_.planar)static_cast<float*>(output->mBuffers[c].mData)[i]=value;
   else static_cast<float*>(output->mBuffers[0].mData)[size_t(i)*channels+c]=value;
  }
  return noErr;
 }
 void waitCallbacks(){auto until=std::chrono::steady_clock::now()+std::chrono::seconds(2);while(callbacks_.load(std::memory_order_acquire)||notifications_.load(std::memory_order_acquire)){check(std::chrono::steady_clock::now()<until,"output callback did not quiesce");std::this_thread::sleep_for(std::chrono::milliseconds(1));}}
 void release(){
  running_.store(0,std::memory_order_release);
  if(proc_)AudioDeviceStop(config_.device,proc_);
  while(registered_){auto l=listeners_[--registered_];check(AudioObjectRemovePropertyListener(l.object,&l.address,changed,this)==noErr,"cannot release output property listener");}
  if(proc_){check(AudioDeviceDestroyIOProcID(config_.device,proc_)==noErr,"cannot release output callback");proc_=nullptr;}
  waitCallbacks();
 }
public:
 CoreAudioOutput(PresentationQueue &queue,uint32_t channels,uint32_t rate):config_(configuration()),queue_(queue),source_channels_(channels),clock_(config_.info.latency_frames){
  check(channels>=1&&channels<=config_.info.channels&&rate==config_.info.sample_rate,"output requires matching source/device rate and channels");
  check(running_.is_lock_free()&&lost_.is_lock_free()&&callbacks_.is_lock_free()&&notifications_.is_lock_free(),"output flags require lock-free atomics");
  try{
   check(AudioDeviceCreateIOProcID(config_.device,callback,this,&proc_)==noErr,"cannot register output callback");
   listeners_={Listener{kAudioObjectSystemObject,{kAudioHardwarePropertyDefaultOutputDevice,kAudioObjectPropertyScopeGlobal,kAudioObjectPropertyElementMain}},
    Listener{config_.device,{kAudioDevicePropertyDeviceIsAlive,kAudioObjectPropertyScopeGlobal,kAudioObjectPropertyElementMain}},
    Listener{config_.device,{kAudioDevicePropertyNominalSampleRate,kAudioObjectPropertyScopeGlobal,kAudioObjectPropertyElementMain}},
    Listener{config_.device,{kAudioDevicePropertyBufferFrameSize,kAudioObjectPropertyScopeGlobal,kAudioObjectPropertyElementMain}},
    Listener{config_.stream,{kAudioStreamPropertyVirtualFormat,kAudioObjectPropertyScopeGlobal,kAudioObjectPropertyElementMain}},
    Listener{config_.stream,{kAudioStreamPropertyLatency,kAudioObjectPropertyScopeGlobal,kAudioObjectPropertyElementMain}},
    Listener{config_.device,{kAudioDevicePropertyLatency,kAudioDevicePropertyScopeOutput,kAudioObjectPropertyElementMain}}};
   for(auto &l:listeners_){check(AudioObjectAddPropertyListener(l.object,&l.address,changed,this)==noErr,"cannot watch output device changes");++registered_;}
  }catch(...){try{release();}catch(...){std::terminate();}throw;}
 }
 ~CoreAudioOutput(){try{release();}catch(...){std::terminate();}}
 OutputBackendInfo info() const noexcept override{return config_.info;}
 void start() override{
  check(!running_.load()&&!lost_.load(),"output is running or interrupted");auto now=configuration();
  check(now.device==config_.device&&now.stream==config_.stream&&now.info.sample_rate==config_.info.sample_rate&&now.info.channels==config_.info.channels&&now.info.buffer_frames==config_.info.buffer_frames&&now.info.latency_frames==config_.info.latency_frames&&now.planar==config_.planar,"output device configuration changed");
  if(!proc_)check(AudioDeviceCreateIOProcID(config_.device,callback,this,&proc_)==noErr,"cannot register resumed output callback");
  clock_.beginSegment();running_.store(1,std::memory_order_release);
  if(AudioDeviceStart(config_.device,proc_)!=noErr){running_.store(0,std::memory_order_release);lost_.store(1,std::memory_order_release);queue_.failClosed();throw std::runtime_error("cannot start output device");}
  try{auto until=std::chrono::steady_clock::now()+std::chrono::seconds(2);
   while(!clock_.anchored()){check(!lost_.load()&&std::chrono::steady_clock::now()<until,"output callback did not start");std::this_thread::sleep_for(std::chrono::milliseconds(1));}
   check(!lost_.load()&&!queue_.error(),"output callback failed at startup");
  }catch(...){running_.store(0,std::memory_order_release);lost_.store(1,std::memory_order_release);queue_.failClosed();AudioDeviceStop(config_.device,proc_);waitCallbacks();throw;}

 }
 void stop() override{
  auto active=running_.exchange(0,std::memory_order_acq_rel);
  if(active)check(AudioDeviceStop(config_.device,proc_)==noErr,"cannot stop output device");
  if(proc_){check(AudioDeviceDestroyIOProcID(config_.device,proc_)==noErr,"cannot retire stopped output callback");proc_=nullptr;}
  waitCallbacks();
 }
 uint64_t deviceFrame() override{
  check(running_.load()&&!lost_.load()&&!clock_.error(),"output is stopped or interrupted");
  AudioTimeStamp time{};time.mFlags=kAudioTimeStampSampleTimeValid;
  check(AudioDeviceGetCurrentTime(config_.device,&time)==noErr,"output device clock is unavailable");
  int64_t sampleFrame=0;uint64_t position=0;check(sample(&time,false,sampleFrame)&&clock_.observedFrame(sampleFrame,position),"invalid output device clock");return position;
 }
 void resetClock() override{check(!running_.load(),"output clock reset requires stopped callbacks");waitCallbacks();clock_.reset();}
};
}}
namespace riffine {
OutputBackendInfo probeOutputBackend(){return configuration().info;}
std::unique_ptr<OutputBackend> openOutputBackend(PresentationQueue &queue,uint32_t channels,uint32_t rate){return std::make_unique<CoreAudioOutput>(queue,channels,rate);}
}
#else
namespace riffine {
OutputBackendInfo probeOutputBackend(){throw std::runtime_error("timestamped output backend is not implemented on this host");}
std::unique_ptr<OutputBackend> openOutputBackend(PresentationQueue &,uint32_t,uint32_t){throw std::runtime_error("timestamped output backend is not implemented on this host");}
}
#endif
