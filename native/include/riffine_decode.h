#ifndef RIFFINE_DECODE_H
#define RIFFINE_DECODE_H
#include <stddef.h>
#include <stdint.h>
#ifdef __cplusplus
extern "C" {
#endif
/* Worker-only ABI v1. UTF-8 paths, immutable original source, caller-owned
 * interleaved float32 buffers. Exact source-frame seeking; no callback use.
 * MP3 coordinates honor the pinned decoder's available delay/padding metadata.
 * MP3 seeks replay decoded frames from zero for deterministic reservoir state.
 * Source precision is zero for MP3/unknown; encoding: 1 WAV PCM, 2 FLAC, 3 MP3. */
typedef struct riffine_decoder riffine_decoder;
typedef struct riffine_decoder_info {
 uint32_t channels, sample_rate, source_bits, encoding;
 uint64_t frames, decoded_frames;
} riffine_decoder_info;
int riffine_decoder_open_file(const char *path, riffine_decoder **decoder, char *error, size_t size);
/* Borrowed OS disk resource: POSIX read-only fd or Windows HANDLE, widened to
 * uint64. Retains an owned duplicate/reopen, never closes the caller's handle or
 * changes its file position. Caller supplies an immutable verified resource.
 * Encoded offset reads are bounded to 64 KiB and run only on the decode worker. */
int riffine_decoder_open_handle(uint64_t handle,riffine_decoder **decoder,char *error,size_t size);
int riffine_decoder_info_get(const riffine_decoder *decoder, riffine_decoder_info *info, char *error, size_t size);
int riffine_decoder_seek(riffine_decoder *decoder, uint64_t frame, char *error, size_t size);
int riffine_decoder_read(riffine_decoder *decoder, float *output, uint32_t capacity_frames, uint32_t *produced_frames, char *error, size_t size);
void riffine_decoder_close(riffine_decoder *decoder);
#ifdef __cplusplus
}
#endif
#endif
