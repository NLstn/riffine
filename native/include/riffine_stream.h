#ifndef RIFFINE_STREAM_H
#define RIFFINE_STREAM_H
#include <stddef.h>
#include <stdint.h>
#ifdef __cplusplus
extern "C" {
#endif
/* Experimental worker ABI v1, independent of files/devices/Go. Calls on one
 * instance must be serialized by its owner. None of these calls are realtime.
 * Samples are borrowed, disjoint, interleaved float32; frames are per channel.
 * Input sample rate is preserved. No resampling, fades or latency trim is hidden.
 * Return 0 on success, 1 with caller-owned error text on failure. */
typedef struct riffine_stream riffine_stream;
typedef struct riffine_stream_config {
  uint32_t version; /* must be 1 */
  uint32_t channels; /* 1 or 2 */
  uint32_t sample_rate; /* 8000..192000 */
  uint32_t max_input_frames; /* 1..8192 per process call */
  double speed; /* .25..2 */
  double semitones; /* -12..12 */
} riffine_stream_config;
typedef struct riffine_stream_limits {
  uint32_t max_input_frames;
  uint32_t max_output_frames; /* process capacity, independent of current speed */
  uint32_t max_flush_frames; /* one terminal flush, independent of current speed */
  uint32_t input_latency_frames; /* source frames */
  uint32_t output_latency_frames; /* output frames, in addition to input latency */
} riffine_stream_limits;
int riffine_stream_create(const riffine_stream_config *config, riffine_stream **stream,
  char *error, size_t error_size);
void riffine_stream_destroy(riffine_stream *stream);
int riffine_stream_get_limits(const riffine_stream *stream, riffine_stream_limits *limits,
  char *error, size_t error_size);
/* Parameter changes take effect on the next process call, preserving fractional
 * output-frame carry and DSP history. Only reset discards buffered audio. */
int riffine_stream_configure(riffine_stream *stream, double speed, double semitones,
  char *error, size_t error_size);
/* Consumes all input or rejects the call before processing. Capacity must fit
 * floor(input_frames/speed + fractional carry). Produced frames exclude latency
 * compensation. Zero input is a no-op, not EOF. Invalid samples/capacity/counts
 * leave state unchanged; unexpected DSP failures require reset or recreation. */
int riffine_stream_process(riffine_stream *stream, const float *input, uint32_t input_frames,
  float *output, uint32_t output_capacity, uint32_t *consumed, uint32_t *produced,
  char *error, size_t error_size);
/* EOF: drain once, using final speed, into max_flush_frames capacity. Returns
 * ceil(input_latency/speed)+output_latency tail frames for a nonempty stream.
 * Latency is exposed, not automatically removed; dynamic changes need a host
 * timeline. Repeated flush returns zero; process/configure require reset at EOF. */
int riffine_stream_flush(riffine_stream *stream, float *output, uint32_t output_capacity,
  uint32_t *produced, char *error, size_t error_size);
/* Drop buffered history/tail/frame carry; preserve format and parameters. */
int riffine_stream_reset(riffine_stream *stream, char *error, size_t error_size);
#ifdef __cplusplus
}
#endif
#endif
