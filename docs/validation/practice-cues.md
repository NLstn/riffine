# Count-in, fixed-second rests and metronome (#23)

Rest gaps use **fixed device-time seconds**, independent of playback speed or
BPM. Tempo entry uses **original-recording BPM**; preparation/metronome spacing
scales with the scheduled speed. At 120 recording BPM and 60% speed, clicks run
at 72 practice BPM, while a two-second rest remains two seconds.

## Saved definitions and workflow

Project v5 adds optional `cues` to a saved practice plan. V1–v4 migrate explicitly
without inventing timing options. Old version labels strictly reject new cue
fields. Existing codec metadata, annotations, settings and ladders are preserved.
Only definitions are persisted; playback phase, remaining frames, generations and
repetition counters are never resumed from a downloaded or device-saved document.

Create/edit a speed ladder in the browser and expand **Preparation and timing**.
Independently enable count-in (1–32 beats), rests (0–120 seconds), and a metronome
during the phrase. Enter original BPM (20–400), first downbeat in source seconds,
quarter-note beats per bar (1–16), and click level (0–100%). The defaults are off;
enabling uses four preparation beats, two rest seconds and 70% click level.
**Timing source → Saved beat markers** derives source frames and bar ordinals
in the core from saved beat annotations. Add beat markers with source position,
measure and beat numbers in the workspace; at least two consecutive beats establish
count-in tempo. Missing, coincident, reversed or out-of-meter beats fail before
playback. Original beat annotations stay portable; only an immutable resolved grid
is used for a selected run. Imported inline beat grids also survive editing;
switch the timing source explicitly when replacing them. No tempo is inferred from
speed. Save on this device or download to retain the complete plan.

Count-in runs before Play on a new run, Stop → Play, Restart, or a new selection.
It does not repeat before every loop. Rest is between scheduled phrase traversals,
including a resumed partial phrase; it does not follow the final loop of a finite
stop plan. Hold plans continue repeating with rests at their target speed.
Pause freezes the acknowledged count-in/rest position and resumes exactly the
remaining device frames. Restart replaces it with a fresh preparation phase;
Stop cancels it; seek bypasses preparation and discards the old cue position.
Live speed edits preserve the current count-in's established tempo/remaining
frames and apply the edited speed to subsequent audio. Rest length never changes.

The view shows the current preparation/rest phase and remaining seconds from the
shared snapshot's device frames/rate. React polling does not advance a phase or
repetition. Removing/editing a saved definition leaves an already selected run
unchanged; select it again to use the new definition.

## Scheduling and host boundaries

The shared transport plans at most two output schedules ahead, each labelled
song audio, `count_in` or `rest`. Cue schedules keep a constant source anchor;
the output timeline still uses presented device frames. Separate generation/
sequence gates handle partial, duplicate/stale and complete acknowledgements.
Only eligible full audio traversals produce `traversal_completed`, loop counts
or speed-ladder advancement. Cue completions produce `phase_completed`, including
the original phase's frame length, and leave every practice counter unchanged.

Count-in/rest support and metronome support are optional device capabilities.
Unsupported hosts reject the plan before resetting or starting their device;
they do not silently run a plain loop instead. Browser capabilities require the
actual bounded click-mixer export. Default plans without cues retain their old
scheduling/acknowledgement behavior.

The browser worker renders count-in with the reusable C++/WASM click ABI and
rests with bounded zero blocks. Song metronome windows are queried from the Go
core on the processing/control worker; JavaScript does not compute tempo, accent
policy or future loop speeds. Each query is bounded to a PCM block plus overlapping
click tails (at most 256 onsets). The shared mixer shortens a click's decay to the
selection end, so even a very short phrase does not abruptly cut a click tail.
Callbacks only copy available PCM and publish counters.

The native scheduled C ABI copies at most 32 count-in events and produces bounded
clicked/silent PCM without allocating during reads. Borrowed source cursors remain
untouched during non-audio phases. The persistent metadata IPC retains protocol
v2 and adds cue operation 6 only when the helper advertises `scheduledCues: 1`;
it transfers at most 512 event bytes, not PCM or completion claims. Native control
workers do not yet support the metronome's per-block Go beat-window port and reject
that setting explicitly. They are still not a native presentation-aware live host.

## Evidence and limits

Core tests cover a finite ladder's count-in → audio → rest order, no trailing rest,
fixed rest frames at changing speeds, duplicate/stale acknowledgements, immutable
cue metadata, exact paused phase frames, speed edits, seek/stop and unsupported
hosts before device changes. Project tests cover v5 round-trip, v4 migration,
strict old-label rejection, invalid controls and nested ownership.

`RIFFINE_LIVE_CUES=1 node scripts/check-browser-live-binding.mjs` runs actual
Go/WASM, C++/WASM processing, AudioWorklet copying and the presentation boundary
with synthetic timestamps. It completes the canonical 36-loop ladder with one
completed preparation phase and 35 rests. Each 0.25-second rest is 12000 frames
at 48 kHz across all nine speeds. It pauses/resumes inside both count-in and rest,
checks unchanged counters and round-trips the exported v5 definition. Synthetic
timestamps do not establish physical audio drift or musical quality.

`RIFFINE_LIVE_BEATS=1 node scripts/check-browser-live-binding.mjs` repeats this
36-loop run with saved source beat annotations resolved by the Go core. It
verifies the same fixed rest lengths, paused phase frames and portable definition.

Native worker tests compare count-in samples with the shared mixer, assert exact
silent rest lengths, unchanged following audio, no C++ read allocations and reset
cancellation. Go/native IPC tests cover bounded payloads, rejection without lost
stream alignment, and capability negotiation. Native/WASM click fixtures verify
normal and bounded phrase-end tails within 1e-6 sample tolerance.

Still outstanding: musical timing/listening and click clipping on physical devices,
long browser sessions, Firefox/Safari/Edge checks, mobile adapter support, native
live-device integration, and broader meter/beat-map editing (the current meter
uses quarter-note pulses). A very
short loop can still exceed the existing two-schedule/polling budget and fail
explicitly; cue scheduling does not invent a callback-side repetition engine.

A short local Chromium UI run on 2026-10-13 created a 60–65% plan with four
count-in beats, a 0.2-second rest and metronome enabled. The view showed count-in
with zero completed loops, then completed exactly four song loops and stopped.
This verifies the generated-fixture UI/device path, not subjective listening or
measured physical drift.
The device-saved plan was then reopened with its original identity verified.
Two beat annotations at 0 and 0.5 source seconds were added through the workspace;
selecting **Saved beat markers** preserved the four-beat count-in, 0.2-second rests
and metronome. This second UI run also completed exactly four loops at 65%.
