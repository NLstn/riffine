# Implementation plan

Date: 2026-10-10. Working name: Riffine. Scope approved: core plus CLI only.

## M0 — Foundation

One Go module; native CMake subproject; MIT license with third-party notices;
core tests on Linux/Windows/macOS; native integration on Windows/macOS; contributor
guide and ADR. No npm, UI, account backend, hosting or sync setup.

Acceptance: `go test ./...`, `go vet ./...`, core build, native build and automated
DSP tests pass. CI publishes development artifacts with licenses, not signed releases.

## M1 — Offline practice vertical slice (initial implementation)

Import and fingerprint local integer PCM WAV. Save version-1 project documents.
Set speed/pitch, enable/disable an A/B loop in source seconds, inspect/verify/relink
source audio, generate per-channel waveform peak JSON. Render a selection using
Signalsmith, with miniaudio decode/encode; play through miniaudio and repeat an
enabled loop. Supply a generated-tone command for a reproducible first session.

The Go API is the primary implementation; the CLI presents it. Shared schemas
reject unknown versions/fields and invalid values. Output files are never replaced.
Project metadata does not embed songs. Source changes fail verification. Streaming
fingerprinting and peaks are bounded; offline DSP has an explicit memory cap.

Acceptance: CLI import -> set -> loop -> inspect -> waveform -> render succeeds;
rendered duration matches source selection/speed; pitch remains independent;
stereo and Unicode paths work; malformed inputs, output collisions and cancellation
do not silently succeed. Native playback requires a device smoke/listening test.

## M2 — Interactive CLI and real-time engine

Add a transport state machine and event API: play/pause/seek, position, live
parameters, stop. Implement streaming decode/DSP/device adapters, bounded buffers,
latency compensation and loop crossfades. Replace render-before-play for long songs.
Add decoder capabilities and WAV/FLAC/MP3 parity only after fixture/licensing review.
Add atomic in-place project updates with concurrency protection and crash recovery.

Acceptance: 30-minute practice sessions on Windows WASAPI and macOS CoreAudio;
record underruns, CPU, memory, seek response, device loss/recovery and audible
artifacts at representative rates (0.5, 0.75, 1, 1.5, 2) and shifts (±2, ±7, ±12).
Include very short loops, stereo phase, silence, clipping and repeated commands.

## M3 — Host/API stabilization

Stabilize versioned commands/events and project migrations. Add CLI JSON command
mode only when another host needs it; avoid premature network services. Document
capabilities, cancellation, errors and persistence ownership. Expand waveform
pyramids for long songs and add saved named regions if justified by practice use.

## M4 — Clients (separately authorized future work)

Shared React UI with browser adapter, then Wails Windows/macOS host. Browser DSP
uses client-side AudioWorklet/WASM; project persistence uses browser local storage
facilities, with explicit file relinking. Offline operation is the default.
WinUI and Linux desktop follow only when needed. Account/settings/audio sync is
optional and independent; never add server audio processing to the browser path.

## Release gates

Before a public branded release: name/trademark clearance, musical listening
tests, compatibility and accessibility checks for any UI, codec/dependency license
inventory, third-party notices, platform signing/notarization and release checksums.
These are not blockers to the current source-code repository and CLI prototype.
