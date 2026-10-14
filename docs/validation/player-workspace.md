# Player-first practice workspace

The app starts in a single practice player. Opening a library song uses that
same player; there is no separate library audio element or compact footer player.
The visible Training projects sidebar supports search, verified reopening,
original selection, recovery versions and removal. Browser copies contain project
settings and annotations, not the original audio.

Speed and pitch edits use the existing shared-core `set_parameters` command.
Continuous input is coalesced for 60 ms, with one command in flight and the final
value retained. Commands use the last accepted snapshot's sequence. Transport
polling does not compete with an active edit; normal snapshots still update the
playhead, loop count and ladder speed. Rejected settings restore the accepted
values and surface an error. Source changes clear pending gestures.

Waveform drag selections, clicks, handles and numeric boundaries send `set_loop`
automatically. Coordinates remain in source frames, and the selection is clamped
to a nonempty range. A white playhead and saved marker labels appear over the
waveform. Loop zoom uses exact PCM detail for bounded ranges and pyramid queries
for longer selections. Speed ladders, timing cues, phrase editing, notes, markers,
portable projects, recovery and keyboard preferences remain available.

## Automated verification

- `npm --prefix web run build`
- `node scripts/check-practice-workspace.mjs`: Strict Mode import ownership,
  coalesced speed/pitch while a previous edit is pending, fresh command sequences,
  automatic loop edits, invalid input, rejected-setting restoration, matching
  library originals and resetting sequences on saved-project reopen.
- `node scripts/check-practice-waveform.mjs`: source-frame keyboard precision,
  clamped clicks, A/B handle dragging, drag selection in both directions, synthetic
  click suppression, playhead/marker rendering and disabled gesture controls.
- Existing facade, keyboard, durable-storage, real Go-WASM saved-project and
  offline-cache checks.
- `go test ./...`, `go vet ./...`, and Go formatting verification.

## Browser verification

Verified the generated MP3 in the real player: drag a selection, start continuous
loop playback, change speed to 50% and pitch by a semitone while playing, pause,
save a named phrase, save a device project and reopen it. Checked desktop and
mobile breakpoints for usable controls and overflow. Browser testing verifies
interactive behavior; audio quality still requires musical listening tests.
