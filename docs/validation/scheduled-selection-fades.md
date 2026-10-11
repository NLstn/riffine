# Explicit scheduled selection fades (partial #15/#26)

The shared selection kernel now accepts the exact `EdgeFadeFrames` requested by
core transport. This is an additive worker C ABI setter, exported by the production
WASM target and exposed as an optional Selection/processing-worker create argument.
Existing preview callers retain their established default fade.

The need appears at rates with fractional 5-ms frame counts and short odd ranges:
transport requests `min(outputRate/200, ceil(outputFrames/2))`, while preview's
default is `min(round(sampleRate*.005), floor(outputFrames/2))`. At 44.1 kHz a
normal transport schedule requests 220 frames and preview defaults to 221.
A live host must pass its explicit schedule value rather than silently substitute
the preview default. Both operate in output-frame units at the kernel's common
source/output rate; sample-rate conversion is not added by this change.

`riffine_selection_set_edge_fade_frames` accepts zero (disabled) through
`ceil(outputFrames/2)` before any source input or emitted/flushed output. Invalid
or late settings reject without modifying the cursor. The setter performs no
allocation. Gains remain independent linear edge ramps, with no overlap, trimming,
extra frames or source-range change. Neither JS nor Go reimplements the gain math.

Native and actual production WASM worker tests compare all output samples against
a zero-fade baseline for 16 cases: rates 8/44.1/48/192 kHz and source lengths
1/2/3/4095 frames. They verify exact scheduled gains and unchanged default gains,
with same-platform float tolerance 2e-7. Native tests reject oversized/late/null
settings and check allocation-free processing. WASM tests reject negative,
fractional, nonfinite and oversized fade arguments, with constructor cleanup.
The C-only ABI test exercises the setter on a one-frame selection.

Required validation includes all six native CTests, native Go integration and
full Go tests/vet/gofmt; the existing 20x5 native/WASM selection conformance test;
web production build; and actual Go/WASM+C++/WASM preview pause/resume/cancel/retry.
Default previews remain unchanged. This supplies the scheduled gain control for
live adapter work; it does not enable repeated browser transport or establish
physical listening/latency evidence.
