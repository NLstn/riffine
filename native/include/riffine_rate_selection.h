#ifndef RIFFINE_RATE_SELECTION_H
#define RIFFINE_RATE_SELECTION_H
#include <stddef.h>
#include <stdint.h>
#ifdef __cplusplus
extern "C" {
#endif
/* Shared processing worker: existing latency-trimmed stretch/pitch followed by
 * bounded miniaudio linear+low-pass rate conversion. Original-source input;
 * device-rate output. Calls are serialized, never realtime. No host policy. */
typedef struct riffine_rate_selection riffine_rate_selection;
typedef struct riffine_rate_selection_info {
 uint32_t channels,source_rate,output_rate,resampler_latency_frames;
 uint64_t output_frames,consumed_source_frames;
} riffine_rate_selection_info;
int riffine_rate_selection_create(uint32_t channels,uint32_t source_rate,uint32_t output_rate,uint64_t source_frames,double speed,double pitch,riffine_rate_selection **,char *,size_t);
int riffine_rate_selection_info_get(const riffine_rate_selection *,riffine_rate_selection_info *,char *,size_t);
int riffine_rate_selection_set_edge_fade_frames(riffine_rate_selection *,uint64_t,char *,size_t);
int riffine_rate_selection_push(riffine_rate_selection *,const float *,uint32_t,char *,size_t);
int riffine_rate_selection_read(riffine_rate_selection *,float *,uint32_t,uint32_t *,uint32_t *,char *,size_t);
void riffine_rate_selection_destroy(riffine_rate_selection *);
#ifdef __cplusplus
}
#endif
#endif
