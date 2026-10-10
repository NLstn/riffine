#ifndef RIFFINE_AUDIO_H
#define RIFFINE_AUDIO_H
#include <stdint.h>
#include <stddef.h>
#ifdef __cplusplus
extern "C" {
#endif
/* Offline ABI v1. Paths are UTF-8. Frames refer to source sample rate.
 * Caller owns paths and error buffer. No exceptions cross this boundary.
 * Output path must not exist. Returns 0 on success, 1 on failure.
 * This is not a real-time callback API. */
int riffine_render(const char *input, const char *output, int64_t start,
  int64_t end, double speed, double semitones, char *error, size_t error_size);
int riffine_play(const char *input, int loop, char *error, size_t error_size);
#ifdef __cplusplus
}
#endif
#endif
