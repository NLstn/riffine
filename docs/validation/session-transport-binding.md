# Shared session transport binding

This partial #15/#19/#26 slice routes playback control through the shared session
and bounded JSON binding. It does not enable audio in a production host.

## Contract

- A host injects `LiveDeviceFactory`. Explicit attachment verifies original bytes
  before opening the device, passes the borrowed reader plus inspected metadata,
  and loads saved speed, pitch and source-frame loop boundaries into the engine.
  Capability becomes true only after successful attachment. The request context
  controls opening, not device lifetime.
- One successful session sequence covers edits and transport commands. The session
  maps to a separate contiguous engine sequence, so annotation edits cannot cause
  transport sequence gaps. Invalid fields/bounds are rejected before device work.
- `play`, `pause`, `stop`, `restart`, `recover`, `seek`, parameter and loop edits use
  the existing engine's generation, partial-traversal and edge-fade policies.
  `seek.position` is a source frame; saved A/B bounds remain source seconds.
- Polling has no clock input. Only device presentation acknowledgements move source
  position or count full traversals. Duplicate/stale acknowledgements remain
  harmless. Embedded transport events retain generation/schedule identity;
  bounded session notification overflow exposes dropped counts and a current
  authoritative snapshot.
- Device failure can change live state, but never commits a failed project edit or
  consumes its session sequence. Recovery reloads committed settings and ends
  stopped with no runtime counts/plans. Annotations remain editable during error.
- Detach/relink/bridge teardown quiesce and close the device before closing its
  reader. Failed attachment also closes in that order. A close error still clears
  live capability: the device contract requires quiescence even on error. Relink
  verifies replacement identity before disturbing the previous live resource.
- `DetachTransport` is an ordered, idempotent operation; `CloseTransport` guarantees
  cleanup without consuming a sequence, including at sequence exhaustion.

The binding operations are `attach_transport`, `detach_transport` and
`poll_transport`. Hosts render returned snapshots; they do not duplicate policy.
The Go service's factory is optional. The browser Go worker, CLI and mobile spike
product wrappers currently provide no production device factory.

## Verification

Generated WAV and synthetic presentation ports exercise 48 kHz source / 96 kHz
output mapping, command interleaving, acknowledged completion, duplicate/stale
acknowledgements, seek/pause/stop, failed parameter rollback/recovery, invalid
commands, cancellation, missing identity, factory/reset/rate failures, idempotent
close, close-error quiescence and verified relink ownership.

The full host report includes `liveBinding`: a deterministic JSON trace through
actual `adapters/binding.Session`, using a clearly synthetic device. It covers
attachment, unacknowledged polling, duplicate completion, failed edit/recovery,
pause, event drain, export, detach, reattach and bridge cleanup. Existing native,
Go/WASM, Android and iOS harness checks compare that complete report across hosts.
This is control/binding evidence; simulated presentation is not hardware evidence.

A regression exposed an engine return-order bug: a device failure returned the
snapshot evaluated before the engine entered `error`. Failure paths now change
state before constructing the response, and tests compare returned/current error
snapshots for failed start and acknowledgement drain.

## Remaining work

No production factory attests device presentation yet. Native preview callback
submission counts remain diagnostics. Browser AudioWorklet scheduling, native
hardware clocks, mobile lifecycle/device recovery, audible boundary behavior,
physical-device latency/listening and saved ladder execution through this session
remain pending. No UI clock or synthetic acknowledgement enables production live
capability. No runtime count is written as practice activity.
