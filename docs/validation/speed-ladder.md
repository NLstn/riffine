# Shared speed ladder (issue #22)

`core/ladder` validates a definition and compiles one-based eligible loop ordinals
to speed ratios. Definitions use start/target speed ratios, positive increments in
**percentage points**, positive loops per step and `hold` or `stop` at target.
Target=start has one step. A final increment that would overshoot is clamped to
target. Stop plays the target for the same repetitions as other steps, then stops;
hold keeps playing at target. Definitions are limited to 10,000 steps and 1,000,000
loops per step. Non-finite values, decreasing targets and unsupported speeds fail.
Shortest round-trip decimal representations are evaluated as exact rationals so
binary float representations of 0.7–0.8 do not create an extra target step.

For 60%, +5 points every 4 loops, target 100%:

| Eligible full loops | Speed |
|---|---|
| 1–4 | 60% |
| 5–8 | 65% |
| 9–12 | 70% |
| 13–16 | 75% |
| 17–20 | 80% |
| 21–24 | 85% |
| 25–28 | 90% |
| 29–32 | 95% |
| 33–36 | 100% |

The transport compiles these speeds into its bounded upcoming schedules **before**
the boundary is played. It never changes pitch while applying a ladder. Only its
eligible presented-traversal completion advances runtime progress. Duplicate/stale
acknowledgements and resumed partial traversals cannot advance it. Snapshot progress
exposes step, repetitions, current speed, next speed and generation.

Pause/resume preserves ladder progress. Explicit restart, seek, loop edit, stop or
recovery resets it. Seeking into a selection cannot count its partial remainder.
Disabling looping pauses the plan. A manual speed override pauses the ladder while
playback uses the requested speed; subsequent transport pause/resume preserves that
override. Explicit `resume_plan` restores the ladder speed/progress. `clear_plan`
removes it. Completed stop plans require restart or clearing before playback. Pitch
edits preserve ladder progress while invalidating old-generation output.

## Persistence and CLI

Project v3 persists named definitions with a stable ID and region reference. It
stores no generation, runtime repetition counter or device state. Explicit v1/v2
migration preserves existing source identity, parameters, seconds and annotations;
reading does not rewrite originals. Session `set_practice_plans` accepts a full,
validated collection with owned copies. Removing a referenced region fails until
the referencing plan is removed. Saved definitions and transient runs are distinct.

Create `ladder.json` for a project with an `intro` region:

```json
[{"id":"ladder","label":"60 to 100","regionId":"intro","ladder":{
  "startSpeed":0.6,"targetSpeed":1,"incrementPoints":5,
  "loopsPerStep":4,"endBehavior":"stop"
}}]
```

```sh
bin/riffine plans --project annotated.json --out planned.json --plans ladder.json
bin/riffine plan --project planned.json --id ladder
```

`plan` returns an explicit schedule preview (loop ordinals and speeds); it does not
play or pretend to count a practice session. The browser live session now selects saved definitions by ID; see
saved-plan-runtime.md. Native product wiring remains pending. The engine's `set_plan` command requires an enabled source selection;
the session's saved definition references identify which region to select.

## Evidence and outstanding work

Unit and fake-device tests cover all 36 required boundaries with unchanged pitch,
stop/hold, target=start, clamped increments, decimal step counting, invalid inputs,
duplicate/stale events, partial resume, seek, manual overrides and disabled loops.
The shared host harness emits the complete 36-loop scheduling/completion trace from
native Go, the WASM worker and C archive. Persistence tests cover v2 migration,
owned plans, region references, full round-trip and omission of runtime state. A
CLI generated-tone flow saves and inspects the exact 9-step/36-loop plan.

Outstanding #22 acceptance: production native audible boundary changes, session
live-plan dispatch, browser/mobile audio adapters and workspace controls. Hardware
conformance and listening are not established by synthetic acknowledgements.
