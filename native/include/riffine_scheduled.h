#ifndef RIFFINE_SCHEDULED_H
#define RIFFINE_SCHEDULED_H
#include <stddef.h>
#include <stdint.h>
#ifdef __cplusplus
extern "C" {
#endif
/* Serialized decode/DSP worker ABI v1; never call on a device callback.
 * Opens an immutable UTF-8 source read-only and retains two decoder cursors.
 * The host supplies schedules compiled by Go; this worker never repeats audio,
 * chooses speeds, counts practice loops or acknowledges presentation. Source and
 * output rates must agree. Resource verification and device clocks are host work.
 * At most two selections pending; input reads <=512 and output reads <=8192.
 * Reset/destroy require caller quiescence. Runtime processing errors poison the
 * worker; destroy/recreate. Invalid arguments leave pending schedules unchanged. */
typedef struct riffine_scheduled_worker riffine_scheduled_worker;
typedef struct riffine_scheduled_info {
 uint32_t version,channels,sample_rate,pending;
 uint64_t source_frames,generation,last_sequence;
} riffine_scheduled_info;
typedef struct riffine_schedule {
 uint64_t generation,sequence,start_frame,end_frame,output_start,output_end,edge_fade_frames;
 double speed,semitones;
} riffine_schedule;
typedef struct riffine_scheduled_block {
 uint64_t generation,sequence,output_start;
 uint32_t frames,generated_complete;
} riffine_scheduled_block;
int riffine_scheduled_open(const char *path,riffine_scheduled_worker **worker,char *error,size_t size);
/* Same worker using the already verified borrowed OS source, without opening
 * its path again. Each decoder retains its own duplicate/offset cursor. */
int riffine_scheduled_open_handle(uint64_t handle,riffine_scheduled_worker **worker,char *error,size_t size);
int riffine_scheduled_info_get(const riffine_scheduled_worker *,riffine_scheduled_info *,char *,size_t);
int riffine_scheduled_reset(riffine_scheduled_worker *,uint64_t generation,char *,size_t);
int riffine_scheduled_submit(riffine_scheduled_worker *,const riffine_schedule *,char *,size_t);
/* Zero frames with success means no scheduled selection, never silent repetition.
 * Blocks stay within one schedule. generated_complete is a DSP diagnostic and
 * cannot be used as a device presentation acknowledgement. Output/result do not
 * alias, and output has capacity*channels float32 samples. No buffer retained. */
int riffine_scheduled_read(riffine_scheduled_worker *,float *output,uint32_t capacity,riffine_scheduled_block *,char *,size_t);
void riffine_scheduled_close(riffine_scheduled_worker *);
#ifdef __cplusplus
}
#endif
#endif
