# Continuous browser parameter changes

Ordinary speed/pitch edits now use the optional shared-core `ParameterDevice`
port. The C++ selection configures its existing stream at a drained input block
boundary, retaining DSP history and decoded source position. Already synthesized
output keeps its old mapping through the reported synthesis-latency boundary.
The Go transport splits the presented timeline there, drops stale future work,
and schedules the remainder and lookahead with the accepted parameters. The
worklet, output ring, presentation counter and generation remain active.

The interrupted prefix and remainder cannot count as completed loops, matching
the previous manual-edit policy. The prefix temporarily occupies a third ledger
entry so two traversal slots remain available at a nearby loop boundary. Only
actual presentation acknowledgements advance source position. Requested speed
survives a pause before the audible transition, and a manual speed override pauses
ladder advancement. Failed updates quiesce output before publishing failure;
pending changes and their acknowledgement batches are bounded.

The UI schedules parameter edits immediately and continues coalescing the newest
value while another edit is pending. The processing worker keeps a 4096-frame
reserve (plus at most one bounded processing block) rather than filling the
32768-frame ring with stale audio. A 2048-frame reserve was rejected after a real
browser run showed starvation. The larger reserve preserves reliable playback.

## Before/after browser profile, 2026-10-14

The development-only `web/profile-live-edits.html` runs the actual Go/WASM,
C++/WASM, worker bridge and Web Audio worklet in the Codex in-app browser on
macOS (Chrome/155). Both profiles use the generated 48 kHz, three-second 440 Hz
WAV, looping [0, 2.8), with 12 alternating edits between 75%/+3 semitones and
100%/0 semitones. Baseline sources come from `f94f35101723` in an isolated checkout;
the baseline includes the original 60 ms UI delay, while the updated profile uses
the immediate parameter-edit timer. Source import and initial playback setup are
excluded. Both runs report 5.8 ms base latency and 32 ms output latency.

| Measurement | Before | After |
| --- | ---: | ---: |
| Median command handling, excluding UI delay | 149.7 ms | 10.6 ms |
| 95th percentile command handling | 166.6 ms | 24.3 ms |
| Median time to detected new pitch, including UI delay | 222.4 ms | 195.3 ms |
| 95th percentile time to detected new pitch | 242.2 ms | 199.5 ms |
| Detected silence during the 12 edits, total | 1025.5 ms | 0 ms |
| Playback resets / suspensions | 12 / 12 | 0 / 0 |
| Output underrun frames | 0 | 0 |

The signal measurement uses an unsmoothed 2048-frame FFT sampled every 4 ms.
Detection requires the largest bin to be within 25 Hz of the target and above
-50 dB; silence requires window RMS squared below 0.00001. It measures the audio
graph, including FFT-window and DSP transition delay, rather than acoustic speaker
latency. Silence estimates exclude gaps too short to empty the analysis window.
These are 12-sample local measurements, not cross-device performance guarantees.
The raw rounded measurements are in `live-parameter-profile.json`.

To repeat, build the browser core/DSP assets, run the web development server, open
`/profile-live-edits.html` and press **Profile playback edits**. The baseline route
uses `?uiDelay=60` on the baseline checkout. Do not run both profiles concurrently.

## Validation and retained fallback

- `check-browser-parameters.mjs`: 64 actual DSP WASM cases across 8–96 kHz and
  25–200% speed, plus 40 repeated fractional-speed changes on one cursor. Checks
  source consumption, exact segment duration and final pitched signal.
- `RIFFINE_LIVE_PARAMETERS=1 RIFFINE_LIVE_SOURCE=web/public/practice/demo.wav`
  with `check-browser-live-binding.mjs`: 12 rapid edits through actual Go/WASM
  and DSP/WASM, unchanged generation/worklet, no underruns, pause/resume with the
  latest parameters and subsequent eligible loop completion. Setting
  `RIFFINE_PARAMETER_DELAY=400` also exercises spaced changes across loop boundaries.
- Shared-core tests cover piecewise source interpolation, discarded lookahead,
  interrupted-loop eligibility, early pause, manual ladder overrides, declined
  updates and failure quiescence. Ledger tests reject rewriting already-presented
  prefixes and retain ordered acknowledgements across an edit.
- Existing Go tests/vet/race checks, native CTests, browser preview/selection/rate
  checks, workspace/facade checks and MP3/M4A/Opus live-binding checks remain valid.

Metronome-enabled playback, active count-in/rest cues, pitch-only changes on a
running ladder, rate-converted cursors, very short or already-flushed selections,
and hosts without the optional port retain the suspend/reset fallback. Seeks,
loop-bound edits and transport controls retain their generation-reset behavior.
Continuous edits avoid decoding a compressed track again merely to change its
parameters; ordinary codec seeking at actual loop boundaries is unchanged.
