# Single-range native streaming preview — issue #14

This follow-up connects the bounded DSP to a read-only WAV cursor, native worker,
preallocated queue and miniaudio device callback. The shared core verifies identity
and chooses source bounds/parameters. The CLI exposes `preview --project ...`.
An enabled loop previews once, with fixed saved speed/pitch. Source/project files
are unchanged and no intermediate WAV is written. #14 remains open.

## Reproduce

```sh
sh scripts/build.sh --native
sh scripts/test.sh --native
# Given a CLI-imported PCM WAV project:
./bin/riffine preview --project song.riffine.json
# Exercise the actual null-backend callback without audible output:
./bin/riffine preview --project song.riffine.json --silent
```

Windows uses the PowerShell scripts with `-Native` and `bin/riffine.exe`.
Normal output explicitly requests CoreAudio (macOS), WASAPI (Windows), or ALSA
(Linux); unavailable devices/backends fail. The test mode explicitly requests
miniaudio's null backend. It is never a fallback for normal playback.

## Boundaries and counts

- Source decode blocks: 512 frames. Worker output blocks: 1,024 frames.
- PCM queue: 32,768 mono/stereo source-rate frames. Prefill watermark: 4,096
  frames (or the entire output if shorter). Producer backpressure retains one
  block and waits off the callback. Buffers depend on format/block limits only.
- DSP raw latency is trimmed at fixed speed; source bounds and output counts
  match core rounding. One final zero frame may reconcile floor/round at EOF.
  A 5 ms edge fade reduces abrupt boundaries. This is not click-free loop or
  seek-quality acceptance. No source/output transport mapping is implemented.
- Callback copies available samples, zero-fills missing samples and updates
  lock-free counters. No allocation, locks, logging, decode, DSP, Go, UI or
  control policy occurs in our callback. Platform/conversion internals still
  need realtime instrumentation and target-device measurements.
- `copiedFrames` measures callback submission at source rate, before miniaudio
  conversion/buffering. It is not device-rate hardware presentation and emits
  no traversal/routine events. Trailing silence at EOF is not an underrun.
- Device stop/reroute/interruption reports an error. Ten seconds without copy
  progress is an explicit stall failure. The control thread allows the reported
  backend buffer plus two periods (minimum 50 ms, maximum 2 s) to drain; this is
  conservative buffering, not a presentation clock or quality guarantee.
- Context cancellation terminates the isolated helper, stopping the device via
  process teardown. The synchronous native ABI has no independent cancel token.
- The report is schema/version/count checked, capped at 8 KiB, with no PCM data.

Core identity verification hashes the entire original before opening the helper.
It uses bounded memory but adds source-size-dependent startup I/O. The helper
reopens the verified path, as offline rendering does: callers must keep resources
stable; concurrent source replacement/modification is unsupported. Native startup
milliseconds measure open/configure/prefill/device start only, excluding Go
verification, helper launch and hardware presentation.

## Local evidence

macOS arm64, AppleClang 21, Go 1.27.2; pinned native dependencies unchanged:

- Shared Go fixture selects [12,000, 36,000) at .75 speed/+3 semitones for both
  offline render and preview. Core preview verifies original identity, cancels
  before host calls, reports capability/host errors and leaves state/events intact.
- Native WAV cursor verifies mono/stereo PCM16/24/32, 8/48/192 kHz, speed .25/.75/
  1/2, pitch extremes, selected-range frequency/channel separation, finite/nonsilent
  output, rounded durations, short ranges and stable EOF. Cursor read and callback
  queue hooks observe zero C++ `new` calls. These hooks do not track C allocation,
  aligned allocation, OS or framework internals.
- A sparse generated 30-minute stereo WAV (86,400,000 source frames) yields the
  first 1,024 preview frames after decoding 6,144 source frames, with metadata
  reporting 115,200,000 output frames. It does not decode the whole selection.
  This tests startup bounding, not a 30-minute realtime device session.
- A generated CLI fixture/null-device run reports 32,000 output/copied frames,
  zero underrun frames, 8,192 source frames decoded before starting output, and
  2.307 ms native startup on one local run. No audible hardware output is claimed.
- Go native helper tests cover null output, Unicode paths, exact counts, source
  SHA-256 preservation, no intermediate WAV, rejected out-of-bounds ranges and
  in-flight deadline cancellation. CLI null-preview integration runs in native CI.
- Go tests/vet/race/format and native CTest pass. The source/queue/null-device
  suite passes AddressSanitizer + UndefinedBehaviorSanitizer and ThreadSanitizer.
  No real output device was opened during these automated checks.

CI on macOS/Windows requires native cursor/queue/null-device checks plus the
existing bounded DSP 30-minute regression, Go native integration and CLI preview.
Cross-platform results are recorded on the PR.

## Remaining acceptance

Continuous transport/control IPC, play/pause/seek/live parameter commands, loop
scheduling, generation resets, output-consumed acknowledgements and exact
source/device mapping remain #14/#15. Device-loss paths are implemented as errors
but hot-unplug/interruption behavior needs actual hardware validation. Capture
CoreAudio/WASAPI startup/seek/parameter latency, underruns, CPU/RSS and listening
results for stereo, short loops and extreme settings, including an actual
30-minute playback session. The browser UI remains basic playback; it is not
connected to this native practice pipeline.
