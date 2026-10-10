# Bounded streaming DSP foundation — issue #14

This initial slice provides an independent file/device-free processing ABI and a
bounded native PCM queue. It does not enable live transport in the Go capabilities
or change CLI play. Issue #14 stays open.

## Reproduce

```sh
sh scripts/build.sh --native
sh scripts/test.sh --native
# Native-only checks (also run on macOS/Windows CI):
ctest --test-dir build/native -C Release --output-on-failure --no-tests=error
# Accelerated generated 30-minute source, not realtime playback:
build/native/riffine-stream-test --soak 1800
```

On Windows use `scripts/build.ps1 -Native` / `scripts/test.ps1 -Native`; the
standalone executable is `build/native/Release/riffine-stream-test.exe` with MSVC.
No source song or output media is committed. DSP dependency revisions are unchanged.

## Contract and checks

- C compilation/link/execution of the opaque ABI; mono/stereo 8–192 kHz limits,
  up to 8,192 source frames per process call, speed .25–2 and pitch ±12.
- Generated independent 440/660 Hz channels: speed .25/.5/.75/1/1.3/2, including
  ±12 semitone extremes; finite output, nonsilent steady-state signal, expected
  frequency within 3.5%, and cumulative exact output counts.
- Invalid version/format/block size, speed/pitch, input NaN, overlapping samples,
  insufficient output/flush capacity: explicit errors and no committed frame
  counts/history. Unexpected processing failures poison the instance until reset.
- Live speed/pitch changes on one running instance reach the generated signal
  and preserve segment output counts. Configure retains history and fractional carry; reset removes old audio/carry;
  short input flushes; empty/repeated flush is empty; post-EOF process/configure
  fail until reset. Raw latency and terminal tail are exposed, not hidden/trimmed.
- Queue saturation rejects writes, wrap preserves channels/order, underruns zero
  output and exclude silence from consumed counts. Two threads transfer 200,000
  stereo frames through a 127-frame queue, with different producer/consumer blocks.
- Count C++ `new` during processing and callback queue reads: zero. This hook does
  not instrument C allocation, OS/framework internals or aligned allocation.
  Callback safety additionally follows from the queue's bounded copy/atomic code;
  this does not prove safety of a future platform callback or device adapter.
- Generated 30-minute stereo source, 48 kHz, speed .75, pitch +3, 4,096-frame
  source blocks: exactly 86,400,000 input and 115,200,000 output frames before tail.
  No selection-sized buffers or output file are used.

## Local execution record

macOS arm64, AppleClang 21, Go 1.27.2, pinned native dependencies:

- CTest signal/lifecycle/queue, C ABI and long-session checks pass (about 12 s).
- `sh scripts/test.sh --native`: native integration, all Go tests, vet and
  formatting pass. Native integration uses generated tones and preserves sources.
- Portable FFT builds pass AddressSanitizer + UndefinedBehaviorSanitizer and
  ThreadSanitizer, including the concurrent queue test. The ordinary CMake build
  selects Accelerate on this Mac; no backend/dependency setting changed.
- Accelerated soak: 86,400,000 source frames, 115,200,000 output frames before
  flush, zero observed C++ `new` calls during processing. 4,096-frame input cap,
  16,385-frame process-output cap and 14,400-frame flush cap at 48 kHz.
- A separate soak took 11.352 s. External `ps` sampling every 200 ms collected
  54 RSS observations; after the first three samples all were 7,200 KiB. This is
  sampled process RSS on one run, not a heap profiler or realtime/device budget.
- The soak initially exposed one-frame drift from summing blockwise quotients.
  Cumulative source-frame planning fixes it; the 30-minute regression is mandatory
  in native CTest on macOS and Windows CI.

Cross-platform CI results are recorded on the pull request.

## Outstanding issue acceptance

- Connect a decode/DSP worker, queue and device callback; implement persistent
  helper control IPC and a shared-core streaming transport adapter.
- Long-song startup and an actual 30-minute native playback session.
- Play/pause/seek/stop, cancellation, live parameter commands, generation resets,
  device-loss recovery or explicit errors, and queue acknowledgement backpressure.
- Compensated source/output timing and audible short-loop boundaries; resampling
  when device format differs. Buffer capacity and raw latency are not budgets.
- CoreAudio and WASAPI hardware/listening measurements: startup/seek/parameter
  latency, underruns, CPU and memory for short loops, stereo and extreme settings.
