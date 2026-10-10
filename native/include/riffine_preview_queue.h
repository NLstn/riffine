#ifndef RIFFINE_PREVIEW_QUEUE_H
#define RIFFINE_PREVIEW_QUEUE_H
#include "riffine_pcm_queue.h"
namespace riffine {
/* Single native worker and callback. All state is preallocated. Submission
 * counters deliberately do not claim actual hardware presentation. Trailing
 * silence after the target is not an underrun. No decoding/DSP/policy runs here. */
class PreviewQueue {
  PCMQueue queue_;
  const uint64_t expected_;
  std::atomic<uint64_t> copied_{0}, underruns_{0};
public:
  PreviewQueue(uint32_t frames,uint32_t channels,uint64_t expected):
    queue_(frames,channels),expected_(expected) {
    if(!expected || !copied_.is_lock_free() || !underruns_.is_lock_free())
      throw std::runtime_error("preview requires a positive range and lock-free counters");
  }
  bool write(const float *input,uint32_t frames) noexcept { return queue_.write(input,frames); }
  uint32_t consume(float *output,uint32_t frames) noexcept {
    auto before=copied_.load(std::memory_order_relaxed);
    auto count=queue_.read(output,frames);
    auto after=before+count;
    auto missing=std::min<uint64_t>(frames-count,expected_>after ? expected_-after : 0);
    underruns_.store(underruns_.load(std::memory_order_relaxed)+missing,std::memory_order_relaxed);
    copied_.store(after,std::memory_order_release);
    return count;
  }
  uint64_t copied() const noexcept { return copied_.load(std::memory_order_acquire); }
  uint64_t underruns() const noexcept { return underruns_.load(std::memory_order_relaxed); }
};
}
#endif
