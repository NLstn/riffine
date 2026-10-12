# Foreground practice keyboard controls (partial #19/#21)

The workspace maps Space to Play/Pause, R to Restart and S to Stop. Each uses the
same typed facade/core command as its visible button. Keys use the current
keyboard layout's letter values. No browser-wide or operating-system-wide
shortcuts are installed.

Open Keyboard shortcuts to remap each action to Space or A–Z, remove an individual
binding, or disable the shortcuts together. Save shortcuts applies the complete
validated mapping and stores a version-one preference on this device. Duplicate
nonempty keys are rejected before writing. These host preferences do not change
project audio, settings, annotations or saved runtime state.

Focus handling stays in the client: inputs, textareas, selects, editable ancestors
and accessible text/combobox controls keep their ordinary keys. Native Space
activation is preserved on buttons, summary/disclosure controls, links and
accessible buttons/checkboxes/radios/switches/tabs. Modified keys and composition
are ignored. Matching repeated keydown events are consumed without dispatching
another command. A synchronous operation latch prevents a second command from
rapid separate presses before React renders busy state.

The workspace is the event scope. Global shortcuts remain unavailable. Playback
buttons expose their active bindings through aria-keyshortcuts, and the existing
visible focus ring, labeled selection inputs, speed/pitch fields and seek input
remain keyboard accessible. Disabling shortcuts preserves native button keyboard
activation. Browser/system keys such as Tab, Enter and Escape cannot be remapped.

## Preference and input evidence

`node --experimental-strip-types scripts/check-practice-shortcuts.mjs` checks the
actual settings/parser/key-discovery functions: defaults and remapping, modifiers
and composition, disabled bindings, conflicting mappings without writes, malformed
stored preferences without overwrite, and storage failure. CI runs this alongside
the actual facade ownership tests and the production web build.

A browser check opened Keyboard shortcuts with native Space and typed R/Space
into Project notes, preserving ordinary text entry and leaving playback unattached.
Native Space on Play initiated the actual transport. Full remapping/repeat/reopen
and uninterrupted keyboard playback checks remain pending: automatic approval
review blocked further browser access after a usage-limit failure. There is also
an intermittent browser-host startup interruption under investigation in the
[playback binding slice](browser-live-binding.md). No complete keyboard-only or
screen-reader workflow is claimed by this partial evidence.

## Hands-free and remaining gates

The selected initial input path is a foreground USB/Bluetooth device that emits
ordinary keyboard keys. It can use these mappings without HID permissions or an
independent playback engine. The client keeps no held-key registry that would
remain stuck after a missing keyup; repeat suppression uses each platform event,
and subsequent fresh presses go through the same operation latch. Actual pedal
selection, disconnect/reconnect, repeat behavior and instrument-playing usability
still need target-device evidence. MIDI/HID discovery, global opt-in controls,
speed-step/A/B/next-region/reset actions, complete screen-reader verification and
the desktop production wiring remain outstanding issue work.
