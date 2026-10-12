# Shared foreground practice actions (partial #21)

All actions use the same version-one session commands from buttons and keyboard
bindings. They accept no extra fields and require a live device. The core polls
actual presentation acknowledgements before deriving position or speed; the UI
never substitutes its own cursor/time. Invalid fields and pre-cancelled commands
leave state unchanged. Only successful actions consume a session sequence.

| Command | Behavior | New-install key |
| --- | --- | --- |
| `rewind_phrase` | Seek to active A/full-track start, preserving paused/playing state; reset runtime repetitions | Q |
| `seek_backward` / `seek_forward` | Seek one source second, clamped to active selection, with exclusive B | J / L |
| `set_a` / `set_b` | Capture acknowledged source frame, keep opposite bound, validate then enable loop | A / B |
| `next_region` | Follow saved region order, wrap after last; first region if current selection has no match. Clear ladder, retain manual speed/pitch, keep playing if previously playing; otherwise stop at new A | N |
| `speed_down` / `speed_up` | Change current playback speed by five percentage points, capped at 25–200%; pause a ladder as a manual override | D / F |
| `practice_reset` | Stop, clear runtime, restore 100% speed/zero pitch/full-track loop-disabled settings; preserve saved regions, notes and plan definitions | T |

Existing Space/R/S defaults remain. Remapping/disable, native Space activation,
text-input/composition/modifier guards and held-key suppression apply to every
action. Version-one preferences containing only the original three keys migrate
in memory with all new actions unbound; loading never writes or introduces a
conflict. Explicitly saving stores the complete accepted mapping. This is a
foreground keyboard path; global/system shortcuts remain unavailable.

A/B and speed changes commit their saved settings only after the device accepts
them. Region/reset operations preserve suspension-tail completion events before
changing resource-scoped transport configuration. Cancellation after configuration
installation keeps the project and command sequence unchanged and exposes error/
recovery state. Recover restores the committed settings. Runtime counters remain
absent from project exports.

## Verification

Core tests use synthetic device acknowledgements at 48 kHz source / 96 kHz output:
A/B capture, paused rewind, exclusive-end/clamped seeks, playing region rotation
and wrap, current ladder speed override, reset retaining annotations/definitions,
speed limits, missing regions/extra fields, missing live capability, cancellation
before work and after Load, device failure rollback and recovery.

`RIFFINE_LIVE_ACTIONS=1 node scripts/check-browser-live-binding.mjs` tests the
actual Go/WASM binding, verified reader, C++/WASM processing and production worklet/
host modules. Platform shims supply a synthetic device clock. It verifies A/B,
rewind, bounded seeks, playing region changes, speed steps, ladder pause, and reset/
export preserving definitions. CI retains practice-actions.json. The existing
saved-plan, live profile, facade ownership and shortcut checks remain in CI.

Actual in-app browser checks increased manual speed to 105%, selected the first
saved phrase, changed to the second while playing, paused and rewound, clamped a
forward seek and captured B, then remapped V/Q. Holding V for 800 ms added one
step to 110%; typing V/Q in notes did not deliver actions. Q rewound while paused.
Reset stopped at source zero, restored 100%/zero pitch/full-track loop-disabled
settings, and retained the two phrases and saved notes. Original Space/R/S with
new actions unbound was restored after testing. Labels/shortcuts were inspected
through the accessibility tree; this is not independent screen-reader evidence.

Go tests/vet/gofmt, practice/binding race checks, production web build and actual
facade/shortcut checks passed. Physical USB/Bluetooth keyboard-emulating pedal
selection, disconnect/reconnect and playing-an-instrument usability still need
hardware verification. MIDI/HID discovery and opt-in global shortcuts remain
separate host capability work; neither is advertised by this browser slice.
