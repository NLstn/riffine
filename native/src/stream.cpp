#include "riffine_stream.h"
#include "signalsmith-stretch/signalsmith-stretch.h"
#include <algorithm>
#include <cmath>
#include <cstdio>
#include <memory>
#include <stdexcept>
#include <vector>

struct riffine_stream {
  signalsmith::stretch::SignalsmithStretch<float> dsp{0};
  riffine_stream_config config{};
  riffine_stream_limits limits{};
  std::vector<float> input, output;
  float *inputs[2]{}, *outputs[2]{};
  long double carry = 0;
  uint64_t sourceFrames = 0, outputFrames = 0;
  bool ended = false, nonempty = false, failed = false;
};
namespace {
void require(bool ok, const char *message) { if (!ok) throw std::runtime_error(message); }
void parameters(double speed, double pitch) {
  require(std::isfinite(speed) && speed >= .25 && speed <= 2 &&
    std::isfinite(pitch) && pitch >= -12 && pitch <= 12, "invalid speed or pitch");
}
template<class F> int boundary(char *error, size_t size, F f) noexcept {
  if (error && size) error[0] = 0;
  try { f(); return 0; }
  catch (const std::exception &e) { if (error && size) std::snprintf(error, size, "%s", e.what()); }
  catch (...) { if (error && size) std::snprintf(error, size, "unexpected streaming DSP error"); }
  return 1;
}
void ready(const riffine_stream *s) {
  require(s, "stream is required");
  require(!s->failed, "DSP failed; reset or recreate stream");
  require(!s->ended, "stream is flushed; reset before processing");
}
void disjoint(const float *input, size_t inSamples, float *output, size_t outSamples) {
  if (!inSamples || !outSamples) return;
  auto a = reinterpret_cast<uintptr_t>(input), b = reinterpret_cast<uintptr_t>(output);
  require(a > b ? a-b >= outSamples*sizeof(float) : b-a >= inSamples*sizeof(float),
    "input and output buffers overlap");
}
void interleave(riffine_stream *s, float *output, uint32_t frames) {
  for (uint32_t i=0; i<frames; ++i) for (uint32_t c=0; c<s->config.channels; ++c) {
    float value=s->outputs[c][i];
    require(std::isfinite(value), "non-finite DSP output; reset stream");
    output[size_t(i)*s->config.channels+c]=value;
  }
}
}
int riffine_stream_create(const riffine_stream_config *config, riffine_stream **stream,
  char *error, size_t size) {
  if (stream) *stream=nullptr;
  return boundary(error, size, [&] {
    require(config && stream, "config and result are required");
    require(config->version==1, "unsupported streaming ABI version");
    require(config->channels>=1 && config->channels<=2 && config->sample_rate>=8000 &&
      config->sample_rate<=192000 && config->max_input_frames>=1 &&
      config->max_input_frames<=8192, "invalid stream format or block limit");
    parameters(config->speed, config->semitones);
    auto s=std::make_unique<riffine_stream>();
    s->config=*config;
    s->dsp.presetDefault(config->channels, float(config->sample_rate));
    s->dsp.setTransposeSemitones(config->semitones);
    s->limits={config->max_input_frames, config->max_input_frames*4+1,
      uint32_t(s->dsp.inputLatency()*4+s->dsp.outputLatency()),
      uint32_t(s->dsp.inputLatency()), uint32_t(s->dsp.outputLatency())};
    s->input.resize(size_t(config->max_input_frames)*config->channels);
    auto outFrames=std::max(s->limits.max_output_frames, s->limits.max_flush_frames);
    s->output.resize(size_t(outFrames)*config->channels);
    for (uint32_t c=0; c<config->channels; ++c) {
      s->inputs[c]=s->input.data()+size_t(c)*config->max_input_frames;
      s->outputs[c]=s->output.data()+size_t(c)*outFrames;
    }
    *stream=s.release();
  });
}
void riffine_stream_destroy(riffine_stream *stream) { delete stream; }
int riffine_stream_get_limits(const riffine_stream *s, riffine_stream_limits *limits,
  char *error, size_t size) {
  return boundary(error,size,[&] { require(s && limits,"stream and limits are required"); *limits=s->limits; });
}
int riffine_stream_configure(riffine_stream *s, double speed, double pitch, char *error, size_t size) {
  return boundary(error,size,[&] {
    ready(s); parameters(speed,pitch);
    s->dsp.setTransposeSemitones(pitch);
    if (speed!=s->config.speed) {
      s->carry=s->sourceFrames/static_cast<long double>(s->config.speed)+s->carry-s->outputFrames;
      s->sourceFrames=0; s->outputFrames=0;
    }
    s->config.speed=speed; s->config.semitones=pitch;
  });
}
int riffine_stream_process(riffine_stream *s, const float *input, uint32_t frames,
  float *output, uint32_t capacity, uint32_t *consumed, uint32_t *produced,
  char *error, size_t size) {
  if (consumed) *consumed=0;
  if (produced) *produced=0;
  return boundary(error,size,[&] {
    ready(s); require(consumed && produced && consumed!=produced,"distinct frame results are required");
    require(frames<=s->limits.max_input_frames,"input block exceeds configured limit");
    if (!frames) return;
    require(input,"input samples are required");
    // Compute from cumulative source frames at this speed. Summing per-block
    // quotients loses a frame at integer boundaries during long sessions.
    require(s->sourceFrames+frames<=(uint64_t(1)<<48),"stream frame counter limit; reset required");
    long double exact=(s->sourceFrames+frames)/static_cast<long double>(s->config.speed)+s->carry;
    auto target=uint64_t(std::floor(exact));
    auto count=uint32_t(target-s->outputFrames);
    require(capacity>=count && (output || count==0),"output capacity is insufficient");
    disjoint(input,size_t(frames)*s->config.channels,output,size_t(count)*s->config.channels);
    for (uint32_t i=0;i<frames;++i) for (uint32_t c=0;c<s->config.channels;++c)
      require(std::isfinite(input[size_t(i)*s->config.channels+c]),"non-finite input sample");
    for (uint32_t i=0;i<frames;++i) for (uint32_t c=0;c<s->config.channels;++c)
      s->inputs[c][i]=input[size_t(i)*s->config.channels+c];
    s->failed=true; // Any unexpected exception after processing starts poisons history.
    s->dsp.process(s->inputs,int(frames),s->outputs,int(count));
    interleave(s,output,count);
    s->sourceFrames+=frames; s->outputFrames=target; s->nonempty=true; s->failed=false;
    *consumed=frames; *produced=count;
  });
}
int riffine_stream_flush(riffine_stream *s, float *output, uint32_t capacity,
  uint32_t *produced, char *error, size_t size) {
  if (produced) *produced=0;
  return boundary(error,size,[&] {
    require(s && produced,"stream and frame result are required");
    require(!s->failed,"DSP failed; reset or recreate stream");
    if (s->ended) return;
    auto count=s->nonempty ? uint32_t(std::ceil(s->limits.input_latency_frames/s->config.speed))+
      s->limits.output_latency_frames : 0;
    require(capacity>=count && (output || count==0),"flush capacity is insufficient");
    s->failed=true;
    if (count) { s->dsp.flush(s->outputs,int(count),float(s->config.speed)); interleave(s,output,count); }
    s->ended=true; s->failed=false; *produced=count;
  });
}
int riffine_stream_reset(riffine_stream *s, char *error, size_t size) {
  return boundary(error,size,[&] {
    require(s,"stream is required"); s->failed=true; s->dsp.reset();
    s->carry=0; s->sourceFrames=0; s->outputFrames=0; s->ended=false; s->nonempty=false; s->failed=false;
  });
}
