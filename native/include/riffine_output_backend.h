#ifndef RIFFINE_OUTPUT_BACKEND_H
#define RIFFINE_OUTPUT_BACKEND_H
#include "riffine_presentation_queue.h"
#include <memory>
namespace riffine {
struct OutputBackendInfo {uint32_t sample_rate,channels,buffer_frames,latency_frames;};
/* Serialized control port. stop quiesces callbacks; it does not attest tail
 * presentation. The controller must gate PCM and observe its presented tail
 * before stop when suspending. Destruction releases platform callbacks before
 * borrowed queue state may be destroyed. PCM and backend clocks share a rate. */
class OutputBackend {
public:
 virtual ~OutputBackend()=default;
 virtual OutputBackendInfo info() const noexcept=0;
 virtual void start()=0;
 virtual void stop()=0;
 virtual uint64_t deviceFrame()=0;
 virtual void resetClock()=0;
};
OutputBackendInfo probeOutputBackend();
std::unique_ptr<OutputBackend> openOutputBackend(PresentationQueue &,uint32_t channels,uint32_t sample_rate);
}
#endif
