# C++ streaming DSP in WASM

Date: 2026-10-11. Partial acceptance evidence for #13 and #26.

The probe compiles the production `native/src/stream.cpp` and its public streaming
C ABI with the same immutable Signalsmith revisions as the native helper. It
runs inside a dedicated worker. Only signal checks, counts and timing metadata
cross the worker/UI boundary; PCM remains in C++ linear memory.

## Coverage

Generated 48 kHz stereo signals (440/660 Hz) pass through 512-frame input blocks
at five independent speed/pitch pairs: 0.25x/0, 0.5x/-12, 0.75x/+3,
1.3x/+7 and 2x/+12 semitones. The probe checks consumed input, exact output frame
counts, finite samples, latency-trimmed frequencies within 4 Hz and channel RMS
between 0.02 and 0.3. It checks invalid configuration rejection, reset,
bounded flush and a repeated flush producing no more frames.

A Node worker executes the complete probe twenty times in one module instance.
Every count and latency must match the native executable exactly; channel
frequencies must agree within 4 Hz and RMS within 0.02 across platform FFTs.
The fixed 32 MiB heap must stay constant. Module errors, nonzero probe exits and
worker timeouts fail the check. The WASM build uses pinned Emscripten 4.0.14 SDK
checkout `eff90ca04a3785f571a8095b3a42b63799cf384a`.

## Local evidence

macOS arm64, AppleClang 21, Node 25.2.1: native and twenty worker runs passed.
The raw WASM module was 98,770 bytes and its JavaScript loader 13,939 bytes.
One observation measured module startup at 42.3 ms, first probe at 92.4 ms,
and later probes around 70–102 ms. These are complete five-case fixture times,
not per-command or audio callback deadlines; the heap measurement is linear
memory, not process RSS. Measurements are observations, not release budgets.

The actual Codex in-app browser worker also passed all five cases: startup
3.1 ms, probe 88.8 ms and 33,554,432 bytes of fixed linear memory. Output frame
counts were respectively 192,000, 96,000, 64,000, 36,923 and 24,000. Separate
initial/tail latency is reported rather than counted as user audio.

## Reproduction

Activate an existing Emscripten 4.0.14 SDK, then from the repository root:

```sh
sh scripts/build-wasm-dsp-spike.sh
python3 -m http.server 4185 --bind 127.0.0.1 --directory build/wasm-dsp
```

Open `http://127.0.0.1:4185/` and run processing conformance. The build script
requires an existing toolchain and downloads only the pinned processing sources
through CMake; it never installs an SDK. Generated modules and evidence remain
under ignored `build/`. CI activates the pinned SDK explicitly and uploads its
report and raw timing/size measurements.

## Remaining gates

This proves processing ABI feasibility, not a browser practice player. It does
not provide AudioWorklet/device playback, presentation-aware acknowledgements,
resampling, long-session memory/underrun evidence or listening tests. The probe
allocates on its worker, never on a real-time audio callback. Browser capabilities
and fallback paths still need production adapters under #26. Physical device
and mobile DSP evidence remain part of #13/#27; no roadmap issue is closed by
this probe alone. Existing dependency revisions and third-party notices apply.
