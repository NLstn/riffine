# Browser presentation mapping (partial #15/#19/#26)

Preview now waits for its tracked audio to pass the browser output timestamp on
hosts providing `AudioContext.getOutputTimestamp`. Other preview hosts keep the
existing conservative latency drain. This is a dependency for live transport;
preview still emits no transport acknowledgements, source position or loop count.

## Coordinates and ownership

The AudioWorklet records an immutable entry for every nonempty copied quantum:
context start frame, copied frame count and cumulative PCM start ordinal. It uses
`currentFrame` from the worklet global scope, in the AudioContext sample-rate
domain. Preview already requires AudioContext rate to match the original rate;
no resampling or source/loop policy is introduced by this mapping.

The control thread uses only `getOutputTimestamp().contextTime * sampleRate`,
rounded down, as the exclusive presented-frame boundary. It counts copied PCM
strictly before that boundary. It never counts underrun zeros, the currently
rendered frame, future entries, idle context time, `currentTime`, callback counters
or elapsed UI time. Duplicate/stale timestamps preserve the last count. Browser
startup testing observed a backward estimate (1123 to 592 frames); stale estimates
are ignored until the reported position catches up, without extrapolation.
Invalid/nonfinite clock values fail explicitly.

These coordinates follow the [Web Audio output timestamp specification](https://www.w3.org/TR/webaudio-1.0/#dom-audiocontext-getoutputtimestamp).
The API reports the browser's device stream position; this does not independently
measure a physical speaker, acoustic latency or musical quality.

One callback writer and one control reader share a fixed ring. Entries remain
immutable until completely presented and retired by the reader. Atomic cursor
publication prevents torn 64-bit coordinates; partial entries remain owned.
32-bit ring indices wrap, while coordinates use low/high words with safe-integer
bounds. Each observation reads at most a bounded snapshot of published entries.

The default 1024-entry allocation is 20512 bytes (32-byte metadata + 20 bytes per
entry), independent of selection length. At 48 kHz/128-frame quanta it retains
about 2.73 seconds of copied blocks; at higher rates the retention time decreases.
The control thread samples every 20 ms. That timer requests timestamps and retires
entries; it never advances audio position. If the reader falls behind and the ring
fills, output stops before copying an untracked block. Malformed coordinates or
ring contents also fail closed. The callback adds only bounded numeric operations
and atomic writes, with no allocation, waits, logging, messages or Go calls.

## Preview lifetime

The monitor starts after output setup/resume and runs while processing is active.
After the core preview response (its borrowed-reader/callback teardown fence),
completion waits for the tracked output count to pass the device timestamp. A
10-second lack of presentation progress while draining fails explicitly. The
monitor is closed on success, cancellation, worker failure or workspace teardown;
closing rejects a pending drain wait. AudioContext closes exactly once.

## Validation

Run `node scripts/check-browser-presentation.mjs` and
`node --experimental-strip-types scripts/check-practice-facade.mjs`.

Mapping tests exercise partial quanta, empty quanta, underrun gaps, duplicate/stale
clock observations, invalid clocks, safe-integer bounds, corrupt entries,
exhaustion before copy, ring cursor wrap and coordinates beyond 32 bits. A separate
Node thread runs the actual production worklet class for 8192 blocks / 786432
copied frames against a concurrent control reader. Its timestamps are synthetic.
Facade tests verify that a completed callback-copy report cannot finish preview
until the device timestamp reaches the tail, and cancellation closes its monitor.
Existing Go/WASM+C++ preview, cancellation/retry and queue tests remain required.

Actual in-app browser verification completed the generated 3-second WAV through
the timestamp-enabled preview path, cancelled a 25% whole-source preview, then
completed a 0.25–0.75 second selection at 75%/+3 after retry. This confirms browser wiring, not independent
hardware presentation accuracy or listening quality.

## Remaining transport work

A live adapter still needs schedule/generation mapping, quiescent pause/stop,
seek/recovery and actual repeated-loop/ladder traces. In particular, the
[AudioContext suspend specification](https://www.w3.org/TR/webaudio-1.0/#dom-audiocontext-suspend)
resolves at handoff to hardware; that promise alone must not be treated as proof
that all submitted PCM was presented. Live capability remains false until the
complete core device contract is implemented and validated.
