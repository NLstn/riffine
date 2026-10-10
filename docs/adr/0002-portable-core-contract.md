# ADR 0002: Portable resource ports and a versioned session contract

Status: accepted for the foundation of roadmap issue #12. Date: 2026-10-10.

## Context

ADR 0001 records the CLI/offline implementation. Its path-based project and WAV
operations prevented the same Go rules from running against browser or mobile
resources. The roadmap requires one product engine and early binding experiments.

## Decision

Keep Go product behavior in `core`. Move path interpretation and all file opening,
creation and publication to `adapters/filesystem`. Core project code constructs,
validates, encodes and decodes documents; WAV code uses host-supplied readers.
`practice.Service` receives resource, storage, decoder and optional renderer ports.
The CLI uses the service with the filesystem and native adapters.

`Service.Import`/`Open` create a session. The session owns validation, independent
speed/pitch, source-time loop selection, verified relinking, save-as, waveform
orchestration and offline rendering. Hosts serialize control work, send commands
and display snapshots. JSON bindings use `practice.DecodeCommand`, not a second
validation implementation. No client owns loop counting or routine evaluation.
The command and project versions are independent. Existing version-1 project JSON
and its units remain valid; there is no schema migration in this change.

Resources have host-scoped opaque handles and saved locators. A locator helps the
host find audio; the SHA-256 of the entire original file plus audio dimensions
establish identity. Core never interprets locators as paths. Filesystem references
remain relative where possible, preserving old CLI projects. Portable identity and
host handle persistence will evolve with #16/#17; current metadata remains PCM WAV.

Successful resource opens transfer reader ownership to core, which always closes
readers. Decoders borrow readers. The store bounds reads, and exclusively publishes
complete documents. Filesystem saves stage/sync data beside the destination and
publish through an exclusive hard link. Unsupported filesystems fail explicitly;
there is no overwrite fallback. This is atomic visibility, not a promise of
power-loss recovery or in-place revision control; those belong to #16. Existing
sources/documents remain intact. See [the API contract](../core-api.md) for details.

Core audio operations check cancellation between bounded reads; blocking reads
must be interruptible by the host when required. Session operations run on a
control worker and hold a control mutex for ordering. No core method is called
by an audio callback, and audio never travels through a UI command bridge.

Version-1 commands cover only implemented edits. Capabilities explicitly advertise
WAV inspection/waveforms and optional offline rendering, with live transport false.
A bounded event queue drops oldest state notifications and supplies a current
snapshot plus a dropped count for resynchronization. It must never be used for
playback acknowledgements. Clock, streaming-decoder and output-schedule ports are
provisional contracts for #13–#15; they do not imply live playback exists.

## Consequences and next gates

Experimental Go call sites change: use `project.Decode`/`Encode`, reader-based
`audio.Inspect`/`Peaks`, and `practice.Service` rather than path-based core methods.
Path-based fixture generation is now `filesystem.WriteTone`; `audio.WriteTone`
accepts a writer. The public CLI flags/output/project format remain compatible.

Shared versioned fixtures exercise both memory and filesystem hosts. #13 must
execute those rules in browser/mobile harnesses and measure bindings before a
shell or bridge is selected. #14/#15 must implement bounded streaming, device
position/latency and authoritative traversal acknowledgements. There is no
browser, mobile or realtime hardware support claim from this foundation.
