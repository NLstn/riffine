#ifndef RIFFINE_PCM_QUEUE_H
#define RIFFINE_PCM_QUEUE_H
#include <algorithm>
#include <atomic>
#include <cstdint>
#include <stdexcept>
#include <vector>
namespace riffine {
/* One producer (decode/DSP worker), one consumer (native device callback).
 * Construct/destroy/reset only when both are quiescent. Queue operations allocate
 * nothing, never wait, and contain no platform/Go/policy calls. Counts are frames.
 * A full queue rejects the entire write; an empty queue emits silence. Consumed
 * frames exclude silence, so underruns cannot count as traversal completion.
 * No resampling: device and producer must agree on channel count/sample rate. */
class PCMQueue {
  const uint32_t capacity_, channels_;
  std::vector<float> data_;
  alignas(64) std::atomic<uint64_t> written_{0};
  alignas(64) std::atomic<uint64_t> read_{0};
  static size_t samples(uint32_t capacity, uint32_t channels) {
    if (!capacity || capacity>262144 || channels<1 || channels>2)
      throw std::invalid_argument("invalid PCM queue capacity or channels");
    return size_t(capacity)*channels;
  }
public:
  PCMQueue(uint32_t capacity, uint32_t channels): capacity_(capacity), channels_(channels),
    data_(samples(capacity,channels)) {
    if (!written_.is_lock_free() || !read_.is_lock_free())
      throw std::runtime_error("PCM queue requires lock-free 64-bit atomics");
  }
  bool write(const float *input, uint32_t frames) noexcept {
    if (frames>capacity_ || (!input && frames)) return false;
    auto w=written_.load(std::memory_order_relaxed);
    auto r=read_.load(std::memory_order_acquire);
    if (frames>capacity_-(w-r)) return false;
    for (uint32_t i=0;i<frames;++i) for (uint32_t c=0;c<channels_;++c)
      data_[size_t((w+i)%capacity_)*channels_+c]=input[size_t(i)*channels_+c];
    written_.store(w+frames,std::memory_order_release);
    return true;
  }
  /* Caller provides frames*channels samples; output must not alias producer input. */
  uint32_t read(float *output, uint32_t frames) noexcept {
    if (!output) return 0;
    auto r=read_.load(std::memory_order_relaxed);
    auto w=written_.load(std::memory_order_acquire);
    auto count=uint32_t(std::min<uint64_t>(frames,w-r));
    for (uint32_t i=0;i<count;++i) for (uint32_t c=0;c<channels_;++c)
      output[size_t(i)*channels_+c]=data_[size_t((r+i)%capacity_)*channels_+c];
    std::fill(output+size_t(count)*channels_,output+size_t(frames)*channels_,0.f);
    read_.store(r+count,std::memory_order_release);
    return count;
  }
  void reset() noexcept { written_.store(0); read_.store(0); }
  uint32_t capacity() const noexcept { return capacity_; }
};
}
#endif
