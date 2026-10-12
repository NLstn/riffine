#ifndef RIFFINE_PRESENTATION_QUEUE_H
#define RIFFINE_PRESENTATION_QUEUE_H
#include "riffine_pcm_queue.h"
#include <cmath>
#include <limits>
namespace riffine {
/* Callback-time -> copied-output mapping. Exactly one callback writer and one
 * serialized control reader. Entries publish/release with acquire/release and
 * remain immutable until retired. Construct/reset/destroy only when quiescent.
 * The reader takes a backend DEVICE position, not a timer or submitted count.
 * It counts frames strictly before that position, preserving gaps and ignoring
 * stale backwards observations. This contains no source/loop/practice policy. */
class PresentationTimeline {
 struct Entry {uint64_t device_start,output_start;uint32_t count;};
 const uint32_t capacity_;
 std::vector<Entry> entries_;
 alignas(64) std::atomic<uint64_t> write_{0};
 alignas(64) std::atomic<uint64_t> read_{0};
 std::atomic<uint32_t> error_{0};
 uint64_t copied_=0,device_end_=0;
 uint64_t presented_=0,completed_=0,reader_device_end_=0,timestamp_=0;
 static uint32_t capacity(uint32_t n){if(n<2||n>4096)throw std::invalid_argument("invalid presentation capacity");return n;}
 bool fail(uint32_t code) noexcept {uint32_t expected=0;error_.compare_exchange_strong(expected,code,std::memory_order_release);return false;}
 static constexpr uint64_t maximum=uint64_t(INT64_MAX);
public:
 explicit PresentationTimeline(uint32_t entries=1024):capacity_(capacity(entries)),entries_(capacity_){
  if(!write_.is_lock_free()||!read_.is_lock_free()||!error_.is_lock_free())throw std::runtime_error("presentation requires lock-free counters");
 }
 // Sole callback writer must call before consuming PCM. A full/corrupt timeline
 // fails before copying, so an unrecorded output range can never be acknowledged.
 bool canAppend(uint64_t device_start,uint32_t count) noexcept {
  if(error())return false;
  if(!count||count>8192||device_start<device_end_||device_start>maximum-count||copied_>maximum-count)return fail(2);
  auto written=write_.load(std::memory_order_relaxed),read=read_.load(std::memory_order_acquire);
  if(written==maximum||read>written||written-read>=capacity_)return fail(3);
  return true;
 }
 // After a successful canAppend, count may shrink to the actual nonzero copied
 // prefix. Only this writer changes the producer cursor; the reader frees slots.
 void append(uint64_t device_start,uint32_t count) noexcept {
  auto written=write_.load(std::memory_order_relaxed);
  entries_[size_t(written%capacity_)]={device_start,copied_,count};
  copied_+=count;device_end_=device_start+count;
  write_.store(written+1,std::memory_order_release);
 }
 bool observe(uint64_t device_frame,uint64_t &presented) noexcept {
  presented=presented_;
  if(error())return false;
  if(device_frame>maximum)return fail(4);
  if(device_frame<timestamp_)return true;
  timestamp_=device_frame;
  auto written=write_.load(std::memory_order_acquire),read=read_.load(std::memory_order_relaxed);
  if(read>written||written-read>capacity_)return fail(1);
  auto count=written-read;
  for(uint64_t i=0;i<count;++i){
   const auto entry=entries_[size_t(read%capacity_)];
   if(!entry.count||entry.count>8192||entry.device_start<reader_device_end_||entry.output_start!=completed_||entry.device_start>maximum-entry.count||entry.output_start>maximum-entry.count)return fail(1);
   if(device_frame<=entry.device_start)break;
   presented_=entry.output_start+std::min<uint64_t>(entry.count,device_frame-entry.device_start);
   if(device_frame<entry.device_start+entry.count)break;
   completed_=entry.output_start+entry.count;reader_device_end_=entry.device_start+entry.count;
   read_.store(++read,std::memory_order_release);
  }
  presented=presented_;return true;
 }
 uint64_t presented() const noexcept{return presented_;} // control reader only
 uint32_t error() const noexcept{return error_.load(std::memory_order_acquire);}
 void reset() noexcept {
  write_.store(0);read_.store(0);error_.store(0);
  copied_=device_end_=presented_=completed_=reader_device_end_=timestamp_=0;
 }
};

/* One decode/DSP producer, one callback, one control observer. No allocation,
 * waits, logging, decoder/DSP/Go calls on consume. Backend frame coordinates are
 * in the SAME negotiated rate as PCM; resampling requires a separate mapping.
 * Pause is acknowledged only when a distinct callback gates PCM. The host must
 * then observe device presentation of the copied tail before stopping output.
 * Underrun silence is diagnostic and never advances presented PCM. */
class PresentationQueue {
 enum State:uint32_t {primed,active,pausing,paused,failed};
 PCMQueue queue_;
 PresentationTimeline timeline_;
 const uint32_t channels_;
 std::atomic<uint32_t> state_{primed};
 std::atomic<uint64_t> copied_{0},underruns_{0};
 uint64_t callback_end_=0;
public:
 PresentationQueue(uint32_t frames,uint32_t channels,uint32_t entries=1024):queue_(frames,channels),timeline_(entries),channels_(channels){
  if(!state_.is_lock_free()||!copied_.is_lock_free()||!underruns_.is_lock_free())throw std::runtime_error("presentation queue requires lock-free state");
 }
 bool write(const float *input,uint32_t frames) noexcept {
  if(state_.load(std::memory_order_acquire)==failed||!input||!frames||frames>8192)return false;
  for(size_t i=0;i<size_t(frames)*channels_;++i)if(!std::isfinite(input[i]))return false;
  return queue_.write(input,frames);
 }
 bool start() noexcept {
  auto expected=uint32_t(primed);
  if(state_.compare_exchange_strong(expected,active,std::memory_order_release))return true;
  expected=paused;return state_.compare_exchange_strong(expected,active,std::memory_order_release);
 }
 bool requestPause() noexcept {
  auto expected=uint32_t(active);return state_.compare_exchange_strong(expected,pausing,std::memory_order_release);
 }
 uint32_t consume(float *output,uint32_t frames,uint64_t device_start) noexcept {
  if(!output||!frames||frames>8192){state_.store(failed,std::memory_order_release);return 0;}
  auto state=state_.load(std::memory_order_acquire);
  if(state==failed){std::fill(output,output+size_t(frames)*channels_,0.f);return 0;}
  if(device_start<callback_end_||device_start>uint64_t(INT64_MAX)-frames){
   std::fill(output,output+size_t(frames)*channels_,0.f);state_.store(failed,std::memory_order_release);return 0;
  }
  callback_end_=device_start+frames;
  if(state!=active){
   std::fill(output,output+size_t(frames)*channels_,0.f);
   if(state==pausing){auto expected=uint32_t(pausing);state_.compare_exchange_strong(expected,paused,std::memory_order_release);}
   return 0;
  }
  if(!timeline_.canAppend(device_start,frames)){
   std::fill(output,output+size_t(frames)*channels_,0.f);state_.store(failed,std::memory_order_release);return 0;
  }
  auto before=underruns_.load(std::memory_order_relaxed);
  if(before>uint64_t(INT64_MAX)-frames){std::fill(output,output+size_t(frames)*channels_,0.f);state_.store(failed,std::memory_order_release);return 0;}
  auto count=queue_.read(output,frames);
  auto missing=frames-count;
  underruns_.store(before+missing,std::memory_order_relaxed);
  copied_.store(copied_.load(std::memory_order_relaxed)+count,std::memory_order_release);
  if(count)timeline_.append(device_start,count);
  return count;
 }
 bool observe(uint64_t device_frame,uint64_t &presented) noexcept {
  if(error()){presented=timeline_.presented();return false;}
  if(!timeline_.observe(device_frame,presented)){state_.store(failed,std::memory_order_release);return false;}
  return state_.load(std::memory_order_acquire)!=failed;
 }
 void failClosed() noexcept{state_.store(failed,std::memory_order_release);}
 bool gated() const noexcept{auto state=state_.load(std::memory_order_acquire);return state==primed||state==paused;}
 bool pauseAcknowledged() const noexcept{return state_.load(std::memory_order_acquire)==paused;}
 bool error() const noexcept{return state_.load(std::memory_order_acquire)==failed;}
 uint64_t copied() const noexcept{return copied_.load(std::memory_order_acquire);}
 uint64_t underruns() const noexcept{return underruns_.load(std::memory_order_relaxed);}
 uint32_t timelineError() const noexcept{return timeline_.error();}
 void reset() noexcept {queue_.reset();timeline_.reset();copied_.store(0);underruns_.store(0);callback_end_=0;state_.store(primed);}
};
}
#endif
