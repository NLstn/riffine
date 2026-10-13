# Shared beat timing and click synthesis foundation (#23)

The chosen tempo semantics are **original-recording BPM**. Device beat spacing
scales with the actual scheduled speed. At 120 source BPM, 60% speed produces
72 practice BPM; pitch does not enter the timing calculation. A speed percentage
alone never creates a tempo. The original standalone foundation is now integrated with the shared transport;
see [count-in/rest/metronome integration](practice-cues.md) for capabilities.

`core/timing` validates and clones a version-1 beat grid. Manual grids use 20–400
source BPM, an explicit source-frame origin and 1–16 quarter-note pulses per bar.
Explicit maps accept up to 4096 strictly increasing source-frame/beat-ordinal
markers and do not also specify manual BPM. Only marked beats are emitted;
unmarked beats and tempo beyond the map are not extrapolated. Ordinals are
zero-based, with ordinal modulo pulses-per-bar determining the accent.

Selections use verified source frames, speed 0.25–2 and 8–192 kHz device rates.
The core independently rounds each beat's output position rather than repeatedly
adding a rounded interval. Device-frame queries are half-open, capped at 256 clicks
and cancellable. Oversized windows fail without partial results; adapters can
query smaller windows. Independent windows emit each boundary beat once. Long
songs remain bounded without retaining their PCM or constructing every click.

A finite 1–32 beat count-in uses manual source tempo or the explicit map interval
containing the selection start. At the map edges it uses the nearest measured
interval. That interval must have consecutive beat ordinals; missing markers do
not silently imply a tempo. Count-ins are capped at 120 output seconds. They are
preparation phases, not audio traversals, and must not complete a speed-ladder
repetition when connected to the presentation acknowledgement state machine.

`riffine_click_mix` is the reusable C ABI for native and WASM click rendering.
The core supplies sorted device-frame events and accent flags. The stateless
mixer operates on caller-owned interleaved float32 blocks (at most 8192 frames,
256 events), allocates nothing and makes no device/core/UI calls. It renders
1 kHz pulses and 1.5 kHz accents with a 0.5 ms attack/10 ms envelope. Level 0–1
scales .18/.25 peaks; additive mixing saturates affected samples to [-1,1].
Disabled/empty click windows preserve source samples exactly. Pitch-independent
clicks are mixed after song processing. Include previous onsets whose tails
overlap a block; block partitioning cannot restart their phase.

## Evidence and integration gate

Go tests cover source/device rates, slowed/fast tempos, explicit variable maps,
source ownership, count-in boundaries, cancellation and invalid definitions.
The same timing tests execute natively and with the Go/WASM runner.
A 30-minute timing query verifies independent rounding and no duplicate/missing
beats through bounded windows. This is a timing computation, not a hardware soak.

Native tests verify exact equality across 127-frame partitions, equal stereo
clicks, linear level, finite tail/gain bounds, saturation and rejection before
output mutation. `node scripts/check-browser-clicks.mjs` compares the full native
and WASM output fixture: maximum absolute sample error locally
4.96e-10 (allowed 1e-6), with a fixed 32 MiB WASM heap. The same C++ source is
compiled for both; no click algorithm is copied to JavaScript.

Count-in and fixed-second rest options now persist in project v5. Shared phase
scheduling, browser metronome windows and exact pause/resume are covered in
[the integration record](practice-cues.md). Musical/hardware drift, clipping
listening, native metronome and mobile adapter checks remain acceptance work.
