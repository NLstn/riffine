# ADR 0003: Worker WASM and an in-process control ABI feasibility slice

Status: experimental; issue #13 remains open. Date: 2026-10-10.

## Evidence and decision

The portable-core PR (#34) defines host ports and the shared fixture. This slice
runs the same Go fixture harness as a native executable, in a real browser worker
via Go/WASM, and through an in-process C ABI on macOS. Their project identity,
settings, rejected commands, event sequence, cancellation and fake output
acknowledgement results match. The C ABI also compiles and links an arm64 iOS
simulator caller. The same pure Go harness builds for Android arm64.

Proceed with Go/WASM in a dedicated worker for browser control experiments. Use
an in-process C ABI for mobile control experiments; it avoids assuming that the
helper executable or filesystem exchange can run on a phone. The experiment
exports one fixture runner with explicit output ownership (`RiffineFree`), not a
production application API. No Swift, Kotlin or JavaScript product rules are copied.
Production packaging, lifecycle and platform storage bridges remain #27/#28.

The shared `engine.Traversal` gate emits one source-range completion only after
the adapter acknowledges the scheduled exclusive output end, and ignores
stale/duplicate acknowledgements. The harness schedules an enabled loop on a
fake output adapter. This is an executable control-boundary experiment, not a
live transport, audible loop or routine counter. #15 still owns full transport
state, resets, source/output mapping and interrupted-traversal behavior.

Measured local results support control-message feasibility only. The browser
bundle is about 5.13 MB raw / 1.40 MB gzip. The browser-worker startup and one
fixture run were about 24 ms and 17 ms on this machine. Node worker comparisons
repeat 20 complete fixture runs and measure serialization/worker round trips.
These are not individual command-latency budgets, network timings or realtime
DSP measurements. Go runtime memory and scheduling must stay outside callbacks.
See [the validation record](../validation/cross-host-spike.md) for exact evidence.

## Processing and realtime boundary

The existing pinned C++ DSP library was linked into a native C++ caller and
rendered a generated selection in-process, preserving the source. Its macOS
build selected Signalsmith Linear's Accelerate backend; a direct consumer must
link `Accelerate`, as CMake's target dependency already does. This observation
qualifies ADR 0001's initial portable-FFT preference; no dependency or FFT backend
was changed in this slice.

Go owns project/practice policy. DSP remains shared C/C++ and runs on workers;
preallocated schedules/queues feed device/AudioWorklet processing. No sample
arrays, Go callbacks, UI calls, allocations or policy decisions may enter a
realtime audio callback. The existing native ABI is offline/file-based and does
not prove streaming. The worker Go runtime must not move into AudioWorklet.

## Unresolved gates and architectural limits

- iOS SDK build/link passed, but simulator inventory timed out. No iOS command
  execution, lifecycle or physical-device result is claimed.
- Android SDK tools are present but no NDK or connected device is available. The
  pure Go executable compiled; the Android C ABI did not build. The explicit
  NDK recipe must run before accepting mobile binding feasibility.
- Emscripten/its C++ WASM sysroot is absent. Go/WASM execution does not prove
  C++ DSP/WASM feasibility. Compile and execute the shared DSP in WASM, including
  its transitive FFT configuration, before accepting browser audio delivery.
- Fake acknowledgement exercises a Go completion gate only. Streaming, latency,
  click-free loops, device loss and hardware measurements are #14/#15 gates.

Keep issue #13 open until those acceptance checks are recorded. This initial
harness slice can merge independently with these limits documented. Do not lock a desktop shell, promise mobile/browser audio delivery, or
claim production bindings from build-only evidence. If Android or C++ WASM cannot
meet their constraints, revise the bridge/processing boundary before client work.


## Architecture acceptance follow-up

The executed mobile/runtime and shared C++ WASM follow-ups supersede the original
missing-SDK/runtime observations. [ADR 0005](0005-measured-host-bindings.md)
records the measured binding decisions and completes #13's feasibility scope.
Production application/audio/lifecycle gates remain on their respective issues.
