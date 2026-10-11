# Shared core API v1

This is the implemented foundation for [issue #12](https://github.com/NLstn/riffine/issues/12).
[ADR 0002](adr/0002-portable-core-contract.md) records the boundary decision.
Public Go APIs remain experimental. Bindings and client presentation are separate
hosts; product validation and practice semantics stay in Go.

## Operations and ownership

Construct `practice.Service` with `resource.Resources`, `resource.Store`, and
`audio.Decoder`. Supply `engine.Renderer` for offline rendering and `engine.Previewer` for
single-pass streaming preview when their host adapters are available.
Call `Import(ctx, sourceHandle, documentHandle)` or `Open(ctx, documentHandle)` to
create a session. Import does not save. Open parses/validates the project but does
not open audio: a session may be edited or relinked while its audio is missing.
`ready` means a valid editable document, not verified audio or a running device.

`Session.Dispatch` applies commands. `Snapshot` returns owned value state, including
a copied capabilities slice. `Verify`, `Waveform`, `Render` and `Preview` inspect the original
source identity before processing. `Relink` accepts only matching bytes AND
metadata. `SaveAs` rebases the reference for a destination, serializes a validated
project, and requests exclusive creation; it leaves the current document, state,
sequence and source unchanged. Reopen the destination to use it as a new session.
No operation replaces source audio or an existing project.

Each `Resources.Open` transfers a reader which core closes on all paths. Decoders
borrow it without closing it. `Reader.Size` describes the resource at open time;
hosts should provide stable reads and report errors if the backing resource changes.
The core checks RIFF bounds and the number of bytes fingerprinted. A source handle
is an opaque host string; `Resolve`/`Reference` translate saved locators relative to
a document. Handles must not substitute for content identity across hosts.
SHA-256 covers original bytes including metadata; re-encoding or retagging changes
identity. Decoder cursors (`DataOffset`/`DataBytes`) are never saved session metadata.
Version-1 project metadata still supports only integer PCM WAV; #17 adds codecs.

`Store.Read` rejects documents over its requested limit before materializing them.
The core independently rejects project JSON over 1 MiB. `Store.Create` must publish
all bytes exclusively or leave no partial destination. The filesystem adapter
stages/syncs and links on the destination volume, with permissions 0600. Existing
files and symlinks fail creation. Filesystems without hard-link support currently
report an error. In-place updates, recovery and concurrent revisions are #16.

## Commands, snapshots and errors

The API version is 1. `DecodeCommand(io.Reader)` rejects messages over 4 KiB,
unknown fields and trailing JSON. `Dispatch` validates the version and payload
again, including nonfinite values in direct Go calls. Commands:

```json
{"version":1,"sequence":1,"type":"set_parameters","speed":0.75,"pitch":-2}
{"version":1,"sequence":2,"type":"set_loop","loop":{"enabled":true,"start":0.25,"end":0.75}}
```

`set_parameters` requires speed and/or pitch, preserving the omitted parameter;
`set_loop` requires the complete loop object. Other payload fields are rejected.
Clients can disable a loop using the current boundaries from their snapshot.
Speed is source/output duration ratio (0.25–2), pitch is independent semitones
(-12–12). Bounds, finite values and source identity are validated only by the core.

Each new session starts at sequence 0. Successful commands require the next
contiguous positive sequence. Concurrent commands are serialized; only one command
with a given sequence succeeds. Failed commands/cancellation do not mutate state,
consume a sequence or emit an event. Repeated, stale or skipped sequences return
`out_of_order`; read a snapshot to recover. There is no automatic retry/replay of
side effects. `Relink` is an ordered Go operation that increments the sequence and
emits `source_relinked`; use its resulting snapshot before sending another edit.
Saving, verification, waveform extraction and rendering emit no playback events.
Long I/O runs under the session control lock; cancellation can be requested without
that lock. Hosts should call operations on a worker to keep presentation responsive.

Errors serialize as `{version, code, message}`. Codes are `invalid_command`,
`unsupported_version`, `invalid_project`, `resource_unavailable`, `identity_mismatch`,
`unsupported_capability`, `out_of_order`, `cancelled` and `host_failure`.
Go errors retain their cause (`errors.Is` works for cancellation/deadlines and
filesystem errors). Cancellation is checked before state commits and publication;
a request racing an already committed operation does not undo it. A host must
observe contexts and bound/interrupt its own blocking operations. WAV parsing,
hashing and peaks check cancellation between reader calls.

Capabilities include tested decoder formats, `offlineRender`, optional
`streamPreview` (omitted when unavailable), and `liveTransport`.
The supplied WAV decoder reports `wav-pcm`; live transport is always false in this
slice. Missing render/preview adapters cause explicit capability errors. Browser extension
lists must not override these host capability reports. Do not expose play/pause,
seek or routines through this API until their core behavior is implemented.

## Time, ranges and future device acknowledgement

Saved loop bounds are source seconds. Convert once with round(seconds × source
sample rate) to `[startFrame, endFrame)`: inclusive start, exclusive end. Frame
counts are per channel, regardless of interleaving. The default render covers
`[0, sourceFrames)`; an enabled loop renders one traversal. A disabled loop's
saved boundaries remain available. A valid enabled loop contains at least one
source frame. Speed/pitch changes do not alter saved source times or frame ranges.

An offline selection's output duration is source duration divided by speed;
render completion is not playback completion. Device-rate output frames, queued
buffers, algorithm latency and source frames are separate quantities. `engine.Clock`
returns monotonic elapsed time; wall time or UI timers cannot acknowledge traversal.

The provisional `OutputDevice` port accepts worker-side bounded schedules with a
generation, sequence, source range and device-output range, at the host's configured
output sample rate. It drains bounded ordered acknowledgements of consumed output,
and resets schedules by generation. No sample arrays or Go callbacks pass through
this control API. Adapters execute precompiled work from preallocated queues.
Acknowledgement overflow must fail explicitly and invalidate playback, never drop
loop completions. #14/#15 still need exact generation/reset rules, source/output
mapping, latency compensation, click-free boundaries and failure recovery; these
interfaces must be revised against those measurements before stabilization.

## Events and backpressure

Each successful edit emits `{version, sequence, type:"session_changed"}`. Relink
emits `source_relinked`. `DrainEvents` returns ordered events, `dropped`, and a current
snapshot atomically, then resets the queue and dropped count. Capacity is 32; on
full queues the oldest state notification is replaced. Producers never wait for a
UI subscriber. When `dropped > 0`, replace displayed state with the supplied snapshot;
do not reconstruct every missed edit. A snapshot also supplies state for a newly
attached client. This queue carries no audio or loss-sensitive output acknowledgements.

## Shared conformance fixture format

Fixtures live in `core/practice/testdata`. `session-v1.json` contains:

- `fixtureVersion` (fixture schema) and `apiVersion` (command contract).
- `audio`: generated PCM16 tone name, seconds, Hz, sample rate and channels.
- `steps`: a `command`, optional expected error code, and expected snapshot
  sequence, speed, pitch and loop after the attempt (including rejection).
- `events`: exact ordered notifications expected after draining once.
- `render`: expected source frame range, speed and pitch sent to a recorder.

A host generates or loads that original tone, imports it using its opaque handles,
executes steps against the same Go core, compares snapshots/error codes, drains
and compares events, and renders through a recording adapter. Host-specific locators
and handles are excluded from cross-host trace equality; persisted settings,
versions, source identity and audio dimensions must match. Memory and filesystem
tests execute this fixture now. The fixture is not evidence of WASM/mobile/device
execution; #13 must run the harness through the actual bindings.

## Acceptance evidence

Implemented: filesystem/native CLI wiring; storage-neutral project/WAV operations;
in-memory import/edit/save/reopen/relink/waveform/render orchestration; shared fixture
executed against memory and filesystem hosts; ordered commands, rollback on failure,
resource ownership, cancellation, exclusive writes and bounded event tests.

Validation commands and the native CLI workflow are recorded in
[the foundation validation record](validation/core-contract.md). Live/hardware,
browser and mobile validation remain separate roadmap gates.

## Single-pass streaming preview

`Session.Preview(ctx)` snapshots the current verified selection under the control
lock and sends fixed source-frame bounds/parameters to `engine.Previewer`. An
enabled loop selects one range; preview does not repeat it. Project state,
command sequences and events remain unchanged, including on error/cancellation.
The native desktop adapter starts one isolated helper, and bounds diagnostic
stdout to 8 KiB. It validates report schema, counts, mode and format; core also
checks the source/output dimensions against the project. Context cancellation
terminates the helper. No samples or realtime callbacks pass through Go.

The native worker decodes bounded blocks through the shared streaming DSP and
feeds a preallocated native queue. The device callback only copies PCM, fills
silence and updates lock-free submission/underrun counters. Normal preview uses
a real requested backend; explicit null-device mode is available for diagnostics.
The complete identity hash is verified before streaming, so startup still
includes a source-size-dependent read. The helper reopens the verified local
path, as the existing offline adapter does; concurrent source replacement or
modification is not supported. Hosts must supply stable resources.

`engine.PreviewReport` is diagnostic output, not an acknowledgement. Copied frames
are callback PCM at source rate before native device conversion and buffering;
they cannot drive `engine.Traversal` or routine counters. Native startup time
ends when device start returns and excludes core hashing, process startup and
hardware presentation. A conservative backend buffer drain is not a hardware
clock. Live transport remains false. See
[the preview validation record](validation/streaming-preview.md).

The experimental `engine.Transport` control API provides versioned commands,
loading/ready/playing/paused/stopped/error snapshots and acknowledged traversal
events. Its host contract and reset/count policies are documented in
[core transport validation](validation/core-transport.md). Production sessions
retain `liveTransport: false` until a presentation-aware device adapter is connected.
