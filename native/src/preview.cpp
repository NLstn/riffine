#include "riffine_preview.h"
#include "riffine_preview_queue.h"
#define NOMINMAX
#include "miniaudio.h"
#include <chrono>
#include <cstdio>
#include <memory>
#include <stdexcept>
#include <string>
#include <thread>
#include <vector>
namespace {
using Clock=std::chrono::steady_clock;
void require(bool ok,const char *message) {if(!ok) throw std::runtime_error(message);}
void sourceResult(int result,char *error) {if(result) throw std::runtime_error(error);}
struct CallbackState {
  riffine::PreviewQueue &queue;
  std::atomic<bool> armed{false},deviceLost{false};
  explicit CallbackState(riffine::PreviewQueue &q):queue(q) {
    require(armed.is_lock_free() && deviceLost.is_lock_free(),"device flags must be lock-free");
  }
  static void data(ma_device *device,void *output,const void *,ma_uint32 frames) noexcept {
    auto *state=static_cast<CallbackState*>(device->pUserData);
    state->queue.consume(static_cast<float*>(output),frames);
  }
  static void notification(const ma_device_notification *event) noexcept {
    auto *state=static_cast<CallbackState*>(event->pDevice->pUserData);
    if(state->armed.load(std::memory_order_relaxed) &&
      (event->type==ma_device_notification_type_stopped || event->type==ma_device_notification_type_rerouted ||
       event->type==ma_device_notification_type_interruption_began))
      state->deviceLost.store(true,std::memory_order_release);
  }
};
struct Device {
  ma_context context{};ma_device value{};bool contextReady=false,deviceReady=false;
  CallbackState &state;
  Device(CallbackState &s,unsigned channels,unsigned rate,bool silent):state(s) {
    ma_backend backend=ma_backend_null;
    if(!silent) {
#ifdef _WIN32
      backend=ma_backend_wasapi;
#elif defined(__APPLE__)
      backend=ma_backend_coreaudio;
#else
      backend=ma_backend_alsa;
#endif
    }
    require(ma_context_init(&backend,1,nullptr,&context)==MA_SUCCESS,"requested output backend is unavailable");contextReady=true;
    auto config=ma_device_config_init(ma_device_type_playback);
    config.playback.format=ma_format_f32;config.playback.channels=channels;config.sampleRate=rate;
    config.periodSizeInFrames=512;config.noFixedSizedCallback=MA_TRUE;config.noPreSilencedOutputBuffer=MA_TRUE;
    config.dataCallback=CallbackState::data;config.notificationCallback=CallbackState::notification;config.pUserData=&state;
    if(ma_device_init(&context,&config,&value)!=MA_SUCCESS) {ma_context_uninit(&context);contextReady=false;throw std::runtime_error("cannot open requested output device");}
    deviceReady=true;
  }
  ~Device() {
    state.armed.store(false,std::memory_order_relaxed);
    if(deviceReady) ma_device_uninit(&value);
    if(contextReady) ma_context_uninit(&context);
  }
};
struct Producer {
  std::atomic<bool> stop{false},failed{false};std::string error;std::thread thread;
  ~Producer() {stop.store(true);if(thread.joinable()) thread.join();}
};
}
int riffine_preview(const char *path,int64_t start,int64_t end,double speed,double pitch,int silent,
  riffine_preview_report *report,char *error,size_t size) {
  if(error && size) error[0]=0;
  if(report) *report={};
  try {
    require(report && (silent==0 || silent==1),"report and explicit output mode are required");
    auto began=Clock::now();char text[512];riffine_preview_source *raw=nullptr;
    sourceResult(riffine_preview_open(path,start,end,speed,pitch,&raw,text,sizeof(text)),text);
    std::unique_ptr<riffine_preview_source,decltype(&riffine_preview_close)> source(raw,riffine_preview_close);
    riffine_preview_info info{};sourceResult(riffine_preview_info_get(source.get(),&info,text,sizeof(text)),text);
    riffine::PreviewQueue queue(32768,info.channels,info.output_frames);CallbackState state(queue);
    Device device(state,info.channels,info.sample_rate,silent!=0);
    std::vector<float> block(1024*info.channels);uint64_t prefilled=0;
    // Decode only enough for a bounded startup watermark, never a whole song.
    while(prefilled<std::min<uint64_t>(4096,info.output_frames)) {
      uint32_t count=0;sourceResult(riffine_preview_read(source.get(),block.data(),1024,&count,text,sizeof(text)),text);
      require(count && queue.write(block.data(),count),"cannot prefill preview queue");prefilled+=count;
    }
    sourceResult(riffine_preview_info_get(source.get(),&info,text,sizeof(text)),text);
    report->decoded_before_start=info.decoded_frames;
    Producer producer;
    producer.thread=std::thread([&] {
      try {
        char message[512];
        while(!producer.stop.load()) {
          uint32_t count=0;sourceResult(riffine_preview_read(source.get(),block.data(),1024,&count,message,sizeof(message)),message);
          if(!count) return;
          while(!queue.write(block.data(),count)) {
            if(producer.stop.load()) return;
            std::this_thread::sleep_for(std::chrono::milliseconds(1));
          }
        }
      } catch(const std::exception &e) {producer.error=e.what();producer.failed.store(true,std::memory_order_release);}
      catch(...) {producer.error="unexpected preview worker error";producer.failed.store(true,std::memory_order_release);}
    });
    state.armed.store(true);
    require(ma_device_start(&device.value)==MA_SUCCESS,"cannot start output device");
    report->startup_milliseconds=std::chrono::duration<double,std::milli>(Clock::now()-began).count();
    uint64_t previous=0;auto progressed=Clock::now();
    while(queue.copied()<info.output_frames) {
      if(producer.failed.load(std::memory_order_acquire)) throw std::runtime_error(producer.error);
      require(!state.deviceLost.load(std::memory_order_acquire),"output device stopped, rerouted or interrupted");
      auto copied=queue.copied();
      if(copied!=previous) {previous=copied;progressed=Clock::now();}
      require(Clock::now()-progressed<std::chrono::seconds(10),"preview stalled for 10 seconds");
      std::this_thread::sleep_for(std::chrono::milliseconds(2));
    }
    // Copying into a callback is earlier than hardware presentation. Allow the
    // backend's reported buffer plus two periods to drain; do not emit a loop ack.
    auto &d=device.value;
    auto drain=uint64_t(d.playback.internalPeriodSizeInFrames)*(uint64_t(d.playback.internalPeriods)+2)*1000/
      std::max(1u,d.playback.internalSampleRate);
    require(drain<=2000,"output device buffer exceeds preview drain limit");
    std::this_thread::sleep_for(std::chrono::milliseconds(std::max<uint64_t>(50,drain)));
    require(!state.deviceLost.load(std::memory_order_acquire),"output device lost during drain");
    if(producer.failed.load(std::memory_order_acquire)) throw std::runtime_error(producer.error);
    report->version=1;report->channels=info.channels;report->sample_rate=info.sample_rate;
    report->output_frames=info.output_frames;report->copied_frames=queue.copied();report->underrun_frames=queue.underruns();
    std::snprintf(report->backend,sizeof(report->backend),"%s",ma_get_backend_name(device.context.backend));
    return 0;
  } catch(const std::exception &e) {if(error && size) std::snprintf(error,size,"%s",e.what());return 1;}
  catch(...) {if(error && size) std::snprintf(error,size,"unexpected native preview error");return 1;}
}
