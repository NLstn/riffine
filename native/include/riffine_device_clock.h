#ifndef RIFFINE_DEVICE_CLOCK_H
#define RIFFINE_DEVICE_CLOCK_H
#include <atomic>
#include <cstdint>
#include <stdexcept>
namespace riffine {
/* One timestamped callback writer, one serialized device-clock observer.
 * Re-anchor only when callbacks are quiescent. Native output timestamps refer
 * to hardware submission; the configured device+stream latency delays the
 * presentation boundary. No host elapsed time or copied count is used. */
class DeviceClockMap {
 const uint64_t latency_;
 std::atomic<uint32_t> ready_{0},failed_{0};
 std::atomic<uint64_t> end_{0};
 int64_t anchor_=0;
 uint64_t segment_=0;
 static constexpr int64_t limit=int64_t(1)<<53;
 bool fail() noexcept{failed_.store(1,std::memory_order_release);return false;}
public:
 explicit DeviceClockMap(uint32_t latency):latency_(latency){
  if(latency>192000||!ready_.is_lock_free()||!failed_.is_lock_free()||!end_.is_lock_free())throw std::invalid_argument("invalid device clock latency or atomics");
 }
 bool callbackFrame(int64_t sample,uint32_t frames,uint64_t &position) noexcept{
  if(failed_.load(std::memory_order_acquire)||sample<=-limit||sample>=limit||!frames||frames>8192)return fail();
  if(!ready_.load(std::memory_order_relaxed)){anchor_=sample;ready_.store(1,std::memory_order_release);}
  auto delta=sample-anchor_;
  if(delta<0)return fail();
  auto next=segment_+uint64_t(delta)+latency_;
  if(next<end_.load(std::memory_order_relaxed)||next>uint64_t(INT64_MAX)-frames)return fail();
  position=next;end_.store(next+frames,std::memory_order_release);return true;
 }
 bool observedFrame(int64_t sample,uint64_t &position) const noexcept{
  if(failed_.load(std::memory_order_acquire)||sample<=-limit||sample>=limit)return false;
  position=segment_;
  if(ready_.load(std::memory_order_acquire)&&sample>anchor_){
   auto delta=uint64_t(sample-anchor_);
   if(segment_>uint64_t(INT64_MAX)-delta)return false;
   position=segment_+delta;
  }
  return true;
 }
 void beginSegment() noexcept{segment_=end_.load(std::memory_order_acquire);ready_.store(0);}
 void reset() noexcept{segment_=0;anchor_=0;ready_.store(0);failed_.store(0);end_.store(0);}
 bool anchored() const noexcept{return ready_.load(std::memory_order_acquire)!=0;}
 bool error() const noexcept{return failed_.load(std::memory_order_acquire)!=0;}
};
}
#endif
