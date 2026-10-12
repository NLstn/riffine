#ifndef RIFFINE_PREVIEW_H
#define RIFFINE_PREVIEW_H
#include <stdint.h>
#include <stddef.h>
#ifdef __cplusplus
extern "C" {
#endif
/* Experimental single-range worker preview. No loop/transport/product policy.
 * UTF-8 input; original-source frames [start,end); parameters fixed per call.
 * Cursor calls are serialized, never realtime. Source is opened read-only.
 * Returned PCM is latency-trimmed and has a 5 ms edge fade. Counts are frames.
 * No musical quality, click-free seek or presentation-clock guarantee is made. */
typedef struct riffine_preview_source riffine_preview_source;
typedef struct riffine_preview_info {
  uint32_t channels, sample_rate;
  /* Includes discarded MP3 seek replay as well as selected source reads. */
  uint64_t output_frames, decoded_frames;
} riffine_preview_info;
int riffine_preview_open(const char *input, int64_t start, int64_t end,
  double speed, double semitones, riffine_preview_source **source,
  char *error, size_t error_size);
/* Borrows a read-only encoded handle. Owns only its retained decoder cursor;
 * never seeks/closes the caller handle. Same bounds and worker rules as open. */
int riffine_preview_open_handle(uint64_t handle,int64_t start,int64_t end,
  double speed,double semitones,riffine_preview_source **source,char *error,size_t error_size);
int riffine_preview_info_get(const riffine_preview_source *source, riffine_preview_info *info,
  char *error, size_t error_size);
/* Writes at most capacity (1..8192) frames. Zero produced means EOF. Errors after
 * decoding begins require recreation; no exception crosses the boundary. */
int riffine_preview_read(riffine_preview_source *source, float *output,
  uint32_t capacity, uint32_t *produced, char *error, size_t error_size);
void riffine_preview_close(riffine_preview_source *source);
typedef struct riffine_preview_report {
  uint32_t version, channels, sample_rate;
  uint64_t output_frames, copied_frames, underrun_frames, decoded_before_start;
  double startup_milliseconds;
  char backend[32];
} riffine_preview_report;
/* Blocking native worker+queue+device path. silent=1 explicitly selects null
 * output; silent=0 requires a real backend, with no silent fallback. Copied
 * frames measure callback submission at source rate, not hardware presentation
 * or device-rate traversal completion. Error/stall/device loss fails explicitly.
 * Desktop Go cancellation terminates the isolated helper process. */
int riffine_preview(const char *input, int64_t start, int64_t end,
  double speed, double semitones, int silent, riffine_preview_report *report,
  char *error, size_t error_size);
/* Same one-selection device preview, using the retained original resource. */
int riffine_preview_handle(uint64_t handle,int64_t start,int64_t end,
  double speed,double semitones,int silent,riffine_preview_report *report,char *error,size_t error_size);
#ifdef __cplusplus
}
#endif
#endif
