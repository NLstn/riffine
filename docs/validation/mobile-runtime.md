# Mobile C-to-Go runtime conformance (issue #13)

The same C caller now repeats the full Go fixture 20 times inside one process,
compares complete reports for determinism and frees every returned buffer. Input
is bounded at 1 MiB, embedded NUL bytes are rejected, and repetitions are bounded
at 256. It prints one JSON report to stdout and per-fixture monotonic timings to
stderr. The measurements include tone generation, hashing, JSON, portable session
operations and synthetic transport acknowledgements; they are not a command-only
latency budget or physical audio result.

## Local iOS result, 2026-10-11

An installed iOS 26.2 iPhone 17 Pro simulator executed the Go arm64 archive through
the C ABI using Xcode's iPhoneSimulator26.5 SDK and Go 1.27.2. All 20 reports matched
native Go exactly, including project v3, annotation round-trip, cancellation,
resource ownership, partial pause/resume and the complete 36-loop 60%→100% ladder.

One local observation: first full fixture 7.891 ms, median 0.693 ms, min 0.629 ms,
max 7.891 ms. Simulator process launch plus 20 fixtures measured 369.345 ms.
Archive size was 9,121,136 bytes and linked runner 3,812,160 bytes. These are samples,
not performance budgets, and exclude device audio and production app lifecycle.
The simulator started for this validation was shut down afterward.

```sh
sh scripts/build-host-spike.sh portable
RIFFINE_SPIKE_OUT=build/host-spike/ios sh scripts/build-host-spike.sh ios
python3 scripts/check-ios-spike.py
```

The script requires an available installed iPhone runtime, boots it if needed,
compares reports, writes ignored `report.json`/`metrics.json` evidence and shuts
down only a simulator it booted. SDK/runtime inventory and operations are bounded;
missing runtimes fail explicitly rather than being recorded as passed build tests.
The CI `ios-runtime` job runs this same path. The checker waits for boot completion
for both newly started and already booted simulators, then probes process spawning
with a tiny simulator-targeted C executable before running the fixture. Only simulator
inventory and this harmless startup probe are retried; the actual twenty-call
conformance run retains its 60-second deadline and is never retried. Command
outputs, elapsed times and timeout diagnostics are retained in `simulator.log`,
which CI uploads even when startup or conformance fails.

## Android recipe and CI gate

The Android build script supports arm64 API 24 by default and an explicit amd64
API 24 compiler for x86_64 emulator execution. The `android-runtime` job uses NDK
27.3.13750724, builds both C ABI libraries and executes the x86_64 one in an API 35
emulator. It compares 20 full reports against native Go and retains library/runner
sizes and timings. Emulator audio is disabled: this is a control/binding runtime
gate, never evidence of audio playback. CI must pass before runtime support is
recorded. No NDK was installed locally for this validation.

```sh
ANDROID_NDK_HOME=/path/to/ndk RIFFINE_NDK_HOST_TAG=linux-x86_64 \
  RIFFINE_ANDROID_ARCH=amd64 RIFFINE_SPIKE_OUT=build/host-spike/android-amd64 \
  sh scripts/build-host-spike.sh android
RIFFINE_ANDROID_SERIAL=emulator-5554 python3 scripts/check-android-spike.py
```

For arm64 targets, omit `RIFFINE_ANDROID_ARCH` and point `RIFFINE_SPIKE_OUT` at that
build while choosing the connected target serial. The checker copies only generated
fixture/harness files into a unique temporary directory and removes them afterward;
it never enumerates personal audio or application documents.

Recipes follow the official [Android emulator command-line documentation](https://developer.android.com/studio/run/emulator-commandline)
and the [GitHub-hosted runner SDK inventory](https://github.com/actions/runner-images/blob/main/images/ubuntu/Ubuntu2404-Readme.md).

## Remaining feasibility gates

Real application lifecycle/UI bindings, physical iOS/Android devices, mobile native
DSP playback, C++ DSP/WASM, browser AudioWorklet timing and persistent native
presentation-aware scheduling remain unverified. Simulator/emulator C processes
establish C-to-Go control feasibility; they do not establish production client,
signing, audio interruptions or long-session memory/quality acceptance. #13 remains
open until those relevant architecture gates have evidence.


## Passing CI runtime evidence

Host feasibility run 38127493873 passed twenty complete calls on API 35 x86_64
Android and iOS 26.4/iPhone 17 Pro simulator. Android first/median fixture calls
were 6.489/4.459 ms (library 4,522,320 bytes); iOS first/median were
45.857/1.679 ms (archive 7,321,008 bytes). Both reports matched native Go.
The run retained report/metrics artifacts. These whole-fixture observations are
not per-command budgets, real-device playback or lifecycle acceptance. Android
arm64 was built in CI but remains unexecuted on physical hardware.


## Architecture acceptance follow-up

The executed mobile/runtime and shared C++ WASM follow-ups supersede the original
missing-SDK/runtime observations. [ADR 0005](../adr/0005-measured-host-bindings.md)
records the measured binding decisions and completes #13's feasibility scope.
Production application/audio/lifecycle gates remain on their respective issues.
