#ifndef RIFFINE_SELECTION_H
#define RIFFINE_SELECTION_H
#include <stddef.h>
#include <stdint.h>
#ifdef __cplusplus
extern "C" {
#endif
/* Single-range processing worker ABI v1. No files, devices, Go or product policy.
 * Fixed speed/pitch; source and output share the sample rate. The host feeds
 * exactly source_frames decoded frames in order. Buffers are borrowed and calls
 * serialized, never realtime. PCM is interleaved finite float32, mono/stereo.
 * Processing trims exposed DSP latency, returns round(source_frames/speed)
 * frames, and applies independent 5 ms edge fades without changing the range.
 * There is at most one zero-padded final rounding frame. */
typedef struct riffine_selection riffine_selection;
typedef struct riffine_selection_info {
 uint32_t channels, sample_rate;
 uint64_t output_frames, consumed_source_frames;
} riffine_selection_info;
int riffine_selection_create(uint32_t channels, uint32_t sample_rate,
 uint64_t source_frames, double speed, double semitones,
 riffine_selection **selection, char *error, size_t error_size);
int riffine_selection_info_get(const riffine_selection *, riffine_selection_info *, char *, size_t);
/* Feed 1..512 whole frames only when read reports needs_input. Reject invalid
 * samples/ranges before processing. Never retains the host input buffer. */
int riffine_selection_push(riffine_selection *, const float *input, uint32_t frames, char *, size_t);
/* Drain at most capacity (1..8192) frames. needs_input=1 requests another source
 * block; produced=0 and needs_input=0 means EOF. The outputs must be distinct.
 * Unexpected DSP failures poison the cursor; destroy/recreate to recover. */
int riffine_selection_read(riffine_selection *, float *output, uint32_t capacity,
 uint32_t *produced, uint32_t *needs_input, char *, size_t);
void riffine_selection_destroy(riffine_selection *);
#ifdef __cplusplus
}
#endif
#endif
