# ADR 0005: Measured control bindings and isolated audio workers

Status: accepted for the host architecture of #13. Date: 2026-10-11.
Supersedes ADR 0003's unresolved build/runtime gates. Production client and device
acceptance remains in #15, #20, #26, #27, #28 and #33.

## Evidence

The same Go fixture executes natively, in Go/WASM workers, through a macOS C ABI,
in an iOS arm64 simulator and in an Android x86_64/API 35 emulator. Complete JSON
reports match, including strict command sequencing/rejection, source identity,
versioned project and annotation round-trip, cancellation, borrowed-resource
ownership, partial traversal handling and a deterministic 36-loop speed ladder.
Twenty repeated mobile calls free each returned buffer. Android arm64 also builds;
physical arm64 execution remains unverified. PR #42's complete CI passed.

The production C++ streaming C ABI also builds natively and in WASM using the
same immutable processing revisions. Five generated stereo speed/pitch cases
check exact frame/latency counts and bounded signal-frequency/RMS tolerances.
Twenty worker runs keep a fixed 32 MiB WASM heap. An actual browser worker and
PR #43's native/WASM/mobile CI passed. No processing algorithm was copied into
JavaScript, Swift or Kotlin.

The original and follow-up measurements/recipes are retained in
[cross-host-spike](../validation/cross-host-spike.md),
[mobile-runtime](../validation/mobile-runtime.md) and
[wasm-streaming-dsp](../validation/wasm-streaming-dsp.md).
Whole fixture timings include generated audio, hashing, serialization and worker
scheduling. They are observations, not individual command or audio deadlines.

## Binding decisions

| Host | Control/product rules | Processing/resource/device boundary |
| --- | --- | --- |
| Browser | Pure Go/WASM in a dedicated control worker; versioned commands/events/snapshots | Shared C++/WASM processing on a worker; host-owned local resource/cache handles and bounded AudioWorklet-facing PCM queue |
| iOS | Go arm64 C archive with explicit C-buffer ownership, called from a serialized control worker | Shared native processing C ABI on its own worker; platform document/audio/lifecycle adapters |
| Android | Go C shared library with explicit C-buffer ownership; thin JNI/application control facade | Shared native processing C ABI on its own worker; platform resource/audio/lifecycle adapters |
| Desktop | Existing pure-Go core and bounded native-helper control IPC, retaining MSVC/process isolation from ADR 0004 | Decode/DSP/PCM remain in the native helper and its device worker; UI shell binds only core control operations |

The desktop shell does not determine product policy. A Wails shell is a candidate
for #20; no shell dependency or UI promise is introduced by this ADR. The fixture
runner itself is not the production application ABI. Production bindings need
opaque instance handles, bounded requests/responses, explicit release/cancellation,
error propagation and lifecycle teardown under the existing shared contracts.
Mobile hosts must link their own resources/device adapters; filesystem paths and
helper executable launches are not assumed available on a phone.

## Realtime and ownership boundaries

Go owns transport/practice policy and plans boundaries ahead. Native/WASM workers
own decode, stretch, resampling and bounded PCM queues. Device/AudioWorklet callbacks
copy available samples, fill underruns and update lock-free counters only. They
never call Go/UI, allocate, log, decode or decide practice policy. Go garbage
collection and JSON/worker scheduling remain outside the realtime path.

Native processing retains opaque C instance ownership and caller-owned buffers.
Go C ABI result buffers require explicit release; never retain Go pointers in
native callbacks or let native exceptions cross a C boundary. Browser workers
exchange control/metadata, not PCM through React or the Go command bridge. Queue
format/rate/capacity and callback limits must be negotiated before playback.

Acknowledgements must represent presented device-rate output after DSP/backend
latency compensation. Fixed-preview submission counts are diagnostics and cannot
complete a practice traversal. Generation/sequence filtering and loop/ladder counts
remain in Go. Backpressure, starvation, device loss and cancellation must propagate
as explicit host events with bounded queues; dropped state notifications cannot
be repurposed as audio completion events.

## What this accepts and what remains

#13's feasibility scope is satisfied by executed common rules, matching complete
traces, native and shared DSP/WASM probes, measured limits and these binding
choices. The architecture does not require a rewrite or server-side audio.
Build-only evidence is labelled separately from simulator/emulator execution.

This decision does not accept physical-device audio, application interruptions,
AudioWorklet scheduling, persistent desktop IPC, long-session memory/underrun
budgets, musical quality, accessibility or signing. Those are production gates
on the corresponding roadmap issues. They must be measured on the actual adapters;
mock traversal and accelerated worker tests cannot substitute for presentation
or listening evidence. Host capability flags remain limited to implemented,
tested operations, and the practice service still reports live transport false.
