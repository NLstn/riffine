# Saved practice plans in the live workspace (partial #19/#26)

Save a named phrase, open Create a speed ladder, enter its start/target speed,
percentage-point increment, repetitions and stop/hold behavior, then save the plan.
Use the plan to select its saved source region; Play starts its paused run. The
workspace shows the shared core's step, repetitions, current/next ladder speed and
planned totals. Completed stop plans disable Play until Restart or return to
manual practice. Hold plans keep repeating their target after the planned total.

Definitions remain in project v4. The additive version-one session commands are
`select_practice_plan` with only `planId`, `resume_practice_plan` and
`clear_practice_plan` without extra fields. The Go session resolves the saved ID,
compiles the ladder and converts its referenced source-second region to frames.
`engine.Transport.LoadPlan` validates before touching the device and selects the
region/ladder under one lock. Selection stops previous output and starts a fresh
ready/paused generation; it does not rewrite saved manual speed or A/B settings.
Only successful operations consume a session command sequence.

Snapshots expose an owned `activePracticePlan` definition and transport progress
with `totalSteps`/`totalLoops`. Current runs retain their definition when the saved
collection changes; selecting again uses the new definition. Source reopen resets
plan drafts, metadata and runtime even when the same source hash is imported.
Manual speed changes pause ladder advancement while manual playback can continue.
Resume ladder restores the core's current step. Return to manual practice suspends
output, drains any completed traversal at that boundary, clears runtime and
restores saved manual settings in a ready state. Recovery/detach discard runtime.
Export stores definitions and manual settings, never counts, generation or device
state. React never computes the next step or advances counters from a timer.

## Evidence

Core tests cover all 36 acknowledged loops for 60–100%, +5 percentage points every
four loops; immutable metadata; edits during a run; unknown/missing IDs and
unexpected fields without mutation; cancellation before selection; failed device
reset and recovery; and selection/clear preserving suspension-tail completions.
The transport tests also verify invalid definitions/disabled loops leave a
running device untouched. Existing pause/seek/partial-loop tests remain applicable.

`RIFFINE_LIVE_PLAN=1 node scripts/check-browser-live-binding.mjs` exercises the
actual Go/WASM binding and live factory, C++/WASM processing and production output
modules, with a synthetic device clock. It checks all 36 events and nine exact
speeds, source bounds, no queued-frame completion, definition edits retaining the
current run, manual override/resume, clearing/restoring manual settings, export
without runtime fields, and selecting the updated five-loop definition. Maximum
source read was 32768 bytes. CI retains `saved-plan.json` alongside existing live
profile reports. This harness does not attest physical audio presentation.

Actual in-app browser verification used the generated three-second WAV and a
0.25–0.75 second phrase. Its 60–100% plan reached stopped/completed at step 9 of 9,
36 of 36 loops and 100% speed. Restart began again; applying manual 80% paused the
ladder while playback continued, and Resume ladder restored 60%. Returning to
manual restored full-track 80% playback in ready state. A zero increment failed
with the core's invalid-definition error. Reopening the generated source during
live playback replaced the prior plan/session and cleared the editor draft.

Go tests/vet/gofmt, race checks for engine/session/binding, facade ownership and
shortcut checks, existing browser live integration, and the production web build
passed. Independent screen-reader, physical-device/pedal, long-session and musical
listening checks remain outstanding. Routines, rests/count-in/metronome, practice
history and durable browser persistence remain separate roadmap work.
