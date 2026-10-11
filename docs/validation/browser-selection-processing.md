# Browser selection processing (partial #26)

A browser processing worker now loads the shared C++ selection pipeline compiled
with pinned Emscripten 4.0.14. It is a bounded processing port, independent of
project commands, decoders, devices, UI timers and practice counters.

## Shared processing

`riffine_selection` accepts exactly one source range's decoded PCM in ordered
1–512-frame blocks. Source channels/sample rate and fixed speed/pitch are explicit.
It exposes needs-input and EOF, drains 1–8192 output frames per read, and never
retains a caller buffer. It trims Signalsmith latency, emits the same rounded
range duration as native preview, permits at most one zero-padded rounding frame,
and applies independent 5 ms edge fades without changing source selection units.
All DSP and fade calculations execute in reusable C++.

Native preview now feeds this cursor using its existing WAV/FLAC/MP3 decoder.
That adapter retains source opening/seeking/decoded-frame diagnostics. The public
preview ABI and copied-frame diagnostic semantics are unchanged.

The browser `Selection` adapter owns fixed WASM input/output/error/result buffers
and frees them after destroying the cursor. The module uses a fixed 32 MiB heap,
with memory growth disabled. Its processing worker handles one cursor, serialized
requests and transferred output blocks. It has no audio callback and does not
copy a whole source or retain output proportional to duration. The eventual host
must apply bounded queue/backpressure instead of accumulating drained blocks.

## Build and check

With the existing pinned Emscripten SDK activated:

```sh
sh scripts/build-browser-dsp.sh
node scripts/check-browser-selection.mjs
```

The native reference binary comes from the normal native build. CI also compiles
it without file/device dependencies in `build/wasm-dsp-native/selection-probe` and
passes that path to the check script. The browser build copies the module, WASM
and complete dependency notices to generated, ignored static assets under
`web/public/practice/`. Distribute the notices alongside those assets.

## Evidence

- Six native CTest checks pass, including direct selection processing, existing
  integer-WAV/compressed preview integration and the 30-minute processing soak.
- The direct native selection test observes no C++ allocations during push/read.
  It checks input/read bounds, nonfinite input nonmutation, EOF, first/last fade
  samples, exact rounded frame counts and stereo frequency/RMS.
- The actual production processing worker and WASM module execute through a Node
  worker transport shim, across 20 repetitions of five speed/pitch pairs:
  `(0.25,0)`, `(0.5,-12)`, `(0.75,+3)`, `(1.3,+7)`, `(2,+12)`.
  Output counts are 192000, 96000, 64000, 36923 and 24000 for one second of 48 kHz
  stereo source. Frequency differs from the native FFT backend by less than 4 Hz;
  RMS uses the existing raw-stream conformance tolerance of 0.02 across
  native/WASM toolchains; the metric records the maximum measured difference.
  Local Apple Accelerate and portable FFT reference builds differ by less than
  0.001. The initially tighter 0.001 cross-platform assertion failed on the Linux
  CI toolchain, so the selection test uses the established DSP tolerance while
  retaining exact duration, finite values, fade checks and frequency bounds.
- One-frame selections at 8/48/192 kHz and a `2^40`-frame coordinate range exercise
  rounding and duration-independent memory. Invalid operations do not advance
  source input. A negative zero at a faded edge is valid numerical silence.
- CI runs both existing raw stream and production selection-worker comparisons
  and saves their measurement records. Timings are observations, not budgets.

No AudioWorklet output or presentation clock is connected by this slice. The
practice workspace still disables live playback; `liveTransport` stays false.
Node worker evidence does not attest an actual browser device, physical listening,
mobile recovery or musical quality. Those remain required follow-on work.
