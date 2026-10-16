# Delivery roadmap

The maintained issue roadmap is [#11](https://github.com/NLstn/riffine/issues/11).
The original [implementation plan](implementation-plan.md) is historical context.
The Go core owns product rules and timing; reusable C++ owns processing. Hosts
provide local resources, storage, devices and bindings; React provides gestures
and views. Audio stays local, and an unchecked release gate remains outstanding.

| Gate | Issues | Current work and remaining acceptance |
| --- | --- | --- |
| R1: shared core/live engine | #12–#15, #17 | Portable contracts, streaming/native/WASM kernels and transport exist. Codec/device parity and physical long-session/listening evidence remain. |
| R2: daily desktop practice | #16, #18–#21 | Portable documents, waveforms, shared React practice controls and shortcuts exist. A first native WinUI Windows client, private Go bridge and installer workflow add song/project opening, waveform selection, settings, saving and one-pass preview. Windows build/install/UI acceptance, native continuous transport and full accessibility/hardware workflow remain. |
| R3: advanced routines | #22–#25 | Core speed ladders/saved-plan execution exist. Count-in, fixed-second rest gaps and a core-driven browser metronome now run through presentation acknowledgements. Native workers render preparation/rest phases. Multiple-phrase routines and automatic key rotation are deferred. Local activity, single-phrase export and physical timing/listening remain. |
| R4: browser/mobile | #26–#28 | Browser live playback/codecs and durable local projects exist. Complete production shell caches for offline reload; short Chromium server-off verification passed. Cross-browser/long-session gates and production mobile packages/clients remain. |
| R5: transcription assistance | #29–#32 | Shared tuning/filter/channel/spectrum tools, optional video and evidence-based legacy migration remain. |
| R6: release | #33 | Collect musical/physical-device evidence throughout; supported formats, accessibility, signing/notarization, notices, checksums and recovery docs gate a supported release. |

Recent evidence:

- [Native Windows client scope and outstanding acceptance](validation/native-windows-app.md)

- [Device saves, revisions, recovery and offline browser](validation/browser-persistence-offline.md)
- [Selection handles and annotation controls](validation/browser-selection-controls.md)
- [Source-tempo beat grids and shared click synthesis](validation/beat-timing.md)
- [Count-in, fixed rests and metronome](validation/practice-cues.md)
- [Saved-plan runtime](validation/saved-plan-runtime.md)
- [Native device-rate scheduling](validation/native-scheduled-rates.md)

Rest gaps are confirmed to use fixed seconds; tempo entry means original-recording
BPM, scaled with song speed. On 2026-10-13, multiple-phrase routine composition
and automatic key rotation (#24) were deferred. Current practice stays focused on
one selected phrase, with its saved speed ladder, timing options and manual pitch
control. Revisit routine traversal order when that feature returns to scope.

The next advanced-practice slice is local activity history (#25), followed by
exporting finite single-phrase practice audio. Routine-specific import/export
and multi-stage rendering remain deferred with #24. The history workflow needs
a decision between automatic recording of acknowledged completed repetitions
and explicitly saving a practice session; no recording behavior has been added.
