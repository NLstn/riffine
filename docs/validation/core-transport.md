# Authoritative shared transport (issue #15, first slice)

`core/engine.Transport` is a host-neutral control-worker state machine. It starts
in `loading`; validated metadata loaded through `Load` enters `ready`. Commands
and acknowledgements are serialized; command sequences are contiguous and only
advance on success. The first host implementation is a deterministic test device.
The production session still advertises `liveTransport: false` until a live host
implements and validates this contract. The single-pass native preview is usable
but reports callback submission and cannot implement the presentation contract.

## Playback contract

Transport plans at most two traversals ahead, each with a generation and distinct
schedule sequence. A source selection is half-open source frames. Output spans
are device-rate frames with rate conversion and speed applied, rounded to the
nearest frame (minimum one frame). Pitch never changes the output length.

Only an ordered adapter acknowledgement of **presented** output advances source
position or completes a traversal. Output decoded, queued, or merely copied into
a callback must not advance it. The host subtracts algorithm latency and measured
device buffering before publishing acknowledgements. Its device clock may inform
that calculation; UI timers and polling frequency do not. Partial positions are
interpolated within the schedule, rounded down to a source frame. Resume can
repeat less than one source frame after this quantization.

The adapter's `Suspend` stops consumption and returns all acknowledgements through
the stopped boundary. `Reset` invalidates old queued work and remains suspended;
`Start` begins consuming scheduled work. Failed operations must leave output
suspended. The adapter must not call back into the locked controller. Batches and
pending completion events are bounded at 128. Overflow, acknowledgement regression,
a skipped scheduled boundary, device loss and starvation are errors, never silent
repetition or lost completion messages. Hosts must drain events regularly.

| Operation | Position | Completed loop count | Generation / partial traversal |
|---|---|---|---|
| Load | Selection start | Reset | New generation; command sequence resets |
| Play | Current position; selection start at EOF | Preserve | New generation; resumed partial remains ineligible |
| Pause | Last acknowledged source frame | Preserve | New generation; queued remainder discarded |
| Seek | Requested frame within active selection | Reset | New generation; middle-to-end is ineligible |
| Stop | Selection start | Reset | New generation; no output |
| Restart | Selection start; playing | Reset | New generation; full traversal eligible |
| Loop edit | New selection start | Reset | New generation; preserve playing/paused state |
| Speed/pitch edit | Last acknowledged source frame | Preserve | New generation; interrupted partial ineligible |
| Recover | Selection start; stopped | Reset | New generation; clear error |

A fully acknowledged loop increments once. Duplicated completions, old generations
and already completed schedule sequences cannot increment again. A resumed partial
can finish playback but cannot count; the following complete loop is eligible.
Previously delivered completions remain historical facts across later commands.
Generation and schedule sequence identify them independently of command sequence.

## Boundary policy

Schedules request linear gain ramps of 5 ms at each edge, capped at half the rounded
output span (rounding up). The host applies them after latency trimming and before
device output. Ramps reach zero at each outer edge, do not overlap neighbouring
selections, and do not change frame counts or source selection timing. Very short
spans may be heavily attenuated; a one-frame selection is silent. These are explicit
fades rather than a crossfade that shifts A/B semantics. A production host must
measure and listen to this policy, including short loops and discontinuous endpoints.

## Evidence and remaining work

Tests cover different source/device rates, partial acknowledgements, decode-ahead,
multiple consumed boundaries in one batch, duplicates/stale generations, pause and
resume, parameter edits, seek, restart, stop, EOF replay, one-source-frame loops,
bounded event overflow, malformed ordering, recovery, cancellation and concurrent
commands. The host feasibility harness also executes pause/resume, a partial
traversal, a full loop and duplicate/stale acknowledgements through this same Go
controller; native Go, the WASM worker and C archive compare identical reports.
`go test -race ./core/engine` checks serialization. No clock advancement
can manufacture a completion because the controller has no wall-clock dependency.

Outstanding issue #15 work: connect the controller through the shared session and
bindings; implement persistent native control/PCM schedules and presentation-aware
acknowledgements; validate the same observable contract with CoreAudio/WASAPI and
other supported hosts; measure buffering, latency, short-loop boundary quality and
device interruptions. These tests do not establish hardware playback accuracy.
