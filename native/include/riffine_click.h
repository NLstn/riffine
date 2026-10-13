#ifndef RIFFINE_CLICK_H
#define RIFFINE_CLICK_H
#include <stddef.h>
#include <stdint.h>
#ifdef __cplusplus
extern "C" {
#endif
/* Worker-only, stateless ABI v1. Timing/accent policy is supplied by the shared
 * core. No files, devices, allocation or product rules. Blocks max 8192 frames;
 * caller-owned event windows max 256 sorted onsets. Include onsets before this
 * block when their 10 ms tails overlap it. Output is interleaved float32. */
typedef struct riffine_click_event {
 int64_t output_frame;
 uint32_t accent; /* 0 or 1 */
 uint32_t reserved; /* must be 0 */
} riffine_click_event;
/* Mix bounded 1 kHz clicks (1.5 kHz accent), with a 0.5 ms attack and 10 ms
 * squared-cosine decay. level 0..1; peak gain .18/.25. Saturate final samples
 * to [-1,1]. All inputs validate before mutation. No pitch-shift is applied.
 * Use a zeroed block for count-in; no event for rests. */
int riffine_click_mix(uint32_t version, uint32_t channels, uint32_t output_rate,
 int64_t block_start, uint32_t frames, float level,
 const riffine_click_event *events, uint32_t event_count, float *output,
 char *error, size_t error_size);
/* Same mixer with an exclusive overall output-window end. Truncate each
 * click's envelope smoothly to this boundary, preserving block partitioning.
 * A block and every onset must be within this window. */
int riffine_click_mix_window(uint32_t version,uint32_t channels,uint32_t output_rate,
 int64_t block_start,uint32_t frames,float level,const riffine_click_event *events,
 uint32_t event_count,float *output,int64_t window_end,char *error,size_t error_size);
#ifdef __cplusplus
}
#endif
#endif
