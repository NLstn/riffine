# ADR 0001: Go core and CLI first, native DSP behind a process adapter

Status: accepted for the initial implementation. Date: 2026-10-10.

## Context

Riffine needs independent speed/pitch, local song import, loops, waveform data and
saved projects. The initial client is only a CLI, used to develop the Go library.
Future browser/Wails/WinUI clients must not dictate the core API. Browser audio
must stay entirely client-side; optional cloud services may only manage accounts,
settings and explicitly requested audio-file sync.

## Decision

Use one Go module. Do not create JS workspaces, UI placeholders or server code.
Go owns project validation, source identity, practice parameters and orchestration.
Host-independent Renderer/Player ports isolate processing and device output.
The first renderer is offline and whole-selection, not a real-time block API.

Signalsmith Stretch handles time/pitch processing; miniaudio handles native WAV
decode/encode and device playback. They are complementary layers. Miniaudio's
resampling alone cannot preserve pitch while changing tempo, so it is not an
alternative stretch algorithm. Native code is compiled as a static library with
a small C ABI and exposed through a helper executable. Go launches the helper
without a shell and terminates it on context cancellation.

This keeps C++ toolchains and cgo out of pure-Go development, supports MSVC on
Windows, and contains native crashes. The cost is startup overhead, temporary
audio files and no live parameter changes. File-based PCM exchange is appropriate
for this first offline slice; it must not become the future real-time boundary.

The current C ABI is offline v1: source/output paths, source frame boundaries,
speed and semitones, plus a caller-owned error buffer. Future streaming DSP needs
an opaque instance with configure/process/reset/flush/latency calls, planar float
PCM, explicit consumed/produced frames, and preallocated buffers. A worker feeds
the miniaudio device callback through a bounded queue; the callback must not call Go.

## Alternatives and selection gates

| Choice | Benefit | Cost / decision |
| --- | --- | --- |
| Go -> cgo -> C ABI | Direct control and lower call overhead | C compiler/ABI setup for Go consumers; defer until streaming is justified |
| Go -> native helper (chosen) | Core tests stay portable, MSVC supported, crash isolation | Offline disk I/O and render delay |
| Pure-Go resampling | Simple deployment | Couples pitch and tempo; fails core practice requirement |
| Signalsmith Web Audio adapter | Official WASM/AudioWorklet route | Useful for future browser; no client in this phase |

Use the default portable FFT implementation; do not enable extra FFT vendors or
fast-math flags. Pin source commits, including the transitive Linear library.
Validate duration, frequency, stereo output, short selections, cancellation and
Unicode paths. Listen to guitar chords, picked transients and full mixes before
claiming quality acceptance. Measure real-time underruns only once a streaming
adapter exists. No latency or CPU superiority claim is made from this spike.

## Future hosts

Wails can bind Go application operations; WinUI can use a versioned local IPC API
or native bridge; a CLI may gain an interactive transport. Browser code uses
Web Audio/AudioWorklet plus Signalsmith WASM, and may compile pure Go rules to WASM.
Do not route browser samples through a server or the Wails command bridge.

## Sources

- [Signalsmith source and API](https://github.com/Signalsmith-Audio/signalsmith-stretch)
- [Official Web Audio adapter](https://github.com/Signalsmith-Audio/signalsmith-stretch/tree/main/web)
- [miniaudio API and supported backends](https://github.com/mackron/miniaudio)

The local implementation uses the pinned revisions in native/CMakeLists.txt;
upstream main-branch documentation may change.
