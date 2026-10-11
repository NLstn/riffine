/* Generated signal conformance through the production streaming C ABI.
 * This runs on a worker; no audio/device/UI callbacks are involved. */
#include "riffine_stream.h"
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define RATE 48000
#define BLOCK 512
#define CHANNELS 2
#define SOURCE_FRAMES 48000
#define PI 3.14159265358979323846
struct case_result {
 double speed, pitch, frequency[2], rms[2];
 unsigned output_frames, tail_frames, input_latency, output_latency;
};
struct analysis {
 unsigned index, begin, end, samples, crossings[2];
 double squares[2];
 float previous[2];
};
static void analyze(struct analysis *a, const float *output, unsigned frames) {
 for (unsigned i = 0; i < frames; ++i, ++a->index) {
  if (a->index >= a->begin && a->index < a->end) {
   for (unsigned c = 0; c < CHANNELS; ++c) {
    float value = output[i * CHANNELS + c];
    a->squares[c] += value * (double)value;
    if (a->samples && a->previous[c] <= 0 && value > 0) ++a->crossings[c];
    a->previous[c] = value;
   }
   ++a->samples;
  }
 }
}
static int run_case(double speed, double pitch, struct case_result *result) {
 riffine_stream_config config = {1, CHANNELS, RATE, BLOCK, speed, pitch};
 riffine_stream *stream = NULL;
 char error[512] = "processing invariant failed";
 if (riffine_stream_create(&config, &stream, error, sizeof(error))) { fprintf(stderr, "%s\n", error); return 1; }
 riffine_stream_limits limits;
 float *output = NULL;
 int status = 1;
 if (riffine_stream_get_limits(stream, &limits, error, sizeof(error))) goto done;
 unsigned capacity = limits.max_output_frames > limits.max_flush_frames ? limits.max_output_frames : limits.max_flush_frames;
 output = calloc(capacity * CHANNELS, sizeof(float));
 if (!output) goto done;
 /* Invalid configure must not change parameters or poison a usable stream. */
 if (!riffine_stream_configure(stream, 0, pitch, error, sizeof(error))) goto done;
 if (riffine_stream_reset(stream, error, sizeof(error))) goto done;
 unsigned expected = (unsigned)floor(SOURCE_FRAMES / speed);
 unsigned trim = (unsigned)ceil(limits.input_latency_frames / speed) + limits.output_latency_frames;
 struct analysis a = {0};
 a.begin = trim + RATE / 20;
 a.end = trim + expected - RATE / 20;
 float input[BLOCK * CHANNELS];
 unsigned total = 0;
 for (unsigned offset = 0; offset < SOURCE_FRAMES; ) {
  unsigned frames = SOURCE_FRAMES - offset;
  if (frames > BLOCK) frames = BLOCK;
  for (unsigned i = 0; i < frames; ++i) {
   input[i * CHANNELS] = (float)(0.2 * sin(2 * PI * 440 * (offset + i) / RATE));
   input[i * CHANNELS + 1] = (float)(0.2 * sin(2 * PI * 660 * (offset + i) / RATE));
  }
  unsigned consumed = 0, produced = 0;
  if (riffine_stream_process(stream, input, frames, output, capacity, &consumed, &produced, error, sizeof(error)) || consumed != frames) goto done;
  for (unsigned i = 0; i < produced * CHANNELS; ++i) if (!isfinite(output[i])) goto done;
  analyze(&a, output, produced);
  offset += consumed; total += produced;
 }
 if (total != expected) { fprintf(stderr, "unexpected output frames: %u != %u\n", total, expected); goto done; }
 unsigned tail = 0;
 if (riffine_stream_flush(stream, output, capacity, &tail, error, sizeof(error))) goto done;
 for (unsigned i = 0; i < tail * CHANNELS; ++i) if (!isfinite(output[i])) goto done;
 analyze(&a, output, tail);
 unsigned repeated = 1;
 if (riffine_stream_flush(stream, output, capacity, &repeated, error, sizeof(error)) || repeated != 0) goto done;
 if (!a.samples || a.index < a.end) goto done;
 for (unsigned c = 0; c < CHANNELS; ++c) {
  result->frequency[c] = a.crossings[c] * (double)RATE / a.samples;
  result->rms[c] = sqrt(a.squares[c] / a.samples);
  double expected_hz = (c ? 660 : 440) * pow(2, pitch / 12);
  if (!isfinite(result->rms[c]) || fabs(result->frequency[c] - expected_hz) > 4 || result->rms[c] < .02 || result->rms[c] > .3) { fprintf(stderr, "signal mismatch speed %.2f pitch %.2f channel %u: %.3f Hz rms %.3f\n", speed, pitch, c, result->frequency[c], result->rms[c]); goto done; }
 }
 if (riffine_stream_reset(stream, error, sizeof(error))) goto done;
 unsigned consumed = 0, produced = 0;
 if (riffine_stream_process(stream, input, BLOCK, output, capacity, &consumed, &produced, error, sizeof(error)) || consumed != BLOCK) goto done;
 result->speed = speed; result->pitch = pitch; result->output_frames = total;
 result->tail_frames = tail; result->input_latency = limits.input_latency_frames;
 result->output_latency = limits.output_latency_frames;
 status = 0;
 done:
 if (status) fprintf(stderr, "stream probe failed: %s\n", error);
 free(output); riffine_stream_destroy(stream); return status;
}
int main(void) {
 const double speeds[] = {.25, .5, .75, 1.3, 2};
 const double pitches[] = {0, -12, 3, 7, 12};
 struct case_result results[5] = {0};
 for (unsigned i = 0; i < 5; ++i) if (run_case(speeds[i], pitches[i], &results[i])) return 1;
 fputs("{\"version\":1,\"sampleRate\":48000,\"channels\":2,\"sourceFrames\":48000,\"cases\":[", stdout);
 for (unsigned i = 0; i < 5; ++i) {
  const struct case_result *r = &results[i];
  if (i) putchar(',');
  printf("{\"speed\":%.6f,\"pitch\":%.6f,\"outputFrames\":%u,\"tailFrames\":%u,\"inputLatency\":%u,\"outputLatency\":%u,\"frequency\":[%.6f,%.6f],\"rms\":[%.6f,%.6f]}", r->speed, r->pitch, r->output_frames, r->tail_frames, r->input_latency, r->output_latency, r->frequency[0], r->frequency[1], r->rms[0], r->rms[1]);
 }
 fputs("]}\n", stdout); return 0;
}
