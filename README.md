# Riffine

A local-first, open-source guitar practice engine. **Working name; pre-alpha.**

Riffine is a local-first guitar practice app. The repository contains a Go
library and CLI plus an early React, TypeScript and Vite music-library client.
The client lets a user choose a local music folder and reads common embedded
tags in the browser. Audio files are not uploaded. The Go CLI imports PCM WAV and, with the tested native helper, FLAC/MP3
songs, saves versioned practice projects, sets independent speed/pitch and A/B
loops, extracts waveform peaks, renders practice audio through Signalsmith
Stretch, and plays it through miniaudio.

The browser client indexes local music and supports basic play/pause, seeking
and track navigation. Practice speed/pitch and loop controls are not connected yet. The current CLI playback renders the selection to a temporary
WAV first; parameters are not changed interactively during playback. The native
processing layer now also has an experimental [bounded streaming DSP API](native/include/riffine_stream.h)
and PCM queue. The CLI `preview` command connects these to a native worker and
output device for one selected range, with fixed speed/pitch and no temporary WAV.
See [streaming DSP checks](docs/validation/streaming-dsp.md) and
[preview validation](docs/validation/streaming-preview.md) for remaining transport
and hardware acceptance.

## Build

The core/CLI needs Go 1.24+ and has no external Go dependencies:

```sh
go test ./...
go build -o bin/riffine ./cmd/riffine
```

The local web client requires Node.js and npm:

```sh
cd web
npm install
npm run dev
```

The Vite development server listens on localhost. Folder access and tag reading
run in the browser; audio files are not sent to a backend.

On Windows use `bin/riffine.exe` as the output name.

Render/play also require CMake 3.24+, Git, and a C++17 compiler. On Windows use
Visual Studio with Desktop development with C++; on macOS use Xcode command-line
tools. Put Go and CMake on PATH. Native dependencies are fetched on the first build.

```powershell
./scripts/build.ps1 -Native
./scripts/test.ps1 -Native
```

```sh
sh scripts/build.sh --native
sh scripts/test.sh --native
```

The scripts put both executables in `bin/` and dependency licenses in `licenses/`.
Alternatively set `RIFFINE_AUDIO_HELPER` to the full `riffine-audio` executable path.

## Try a practice session

Use `./bin/riffine.exe` instead of `./bin/riffine` on Windows.

```sh
./bin/riffine tone --out tone.wav --seconds 3
./bin/riffine import --audio tone.wav --out song.riffine.json
./bin/riffine set --project song.riffine.json --out slow.riffine.json --speed 0.75 --pitch -2
./bin/riffine loop --project slow.riffine.json --out loop.riffine.json --start 0.5 --end 2.5
./bin/riffine inspect --project loop.riffine.json --verify
./bin/riffine waveform --project loop.riffine.json --bins 200 --out song.peaks.json
./bin/riffine render --project loop.riffine.json --out practice.wav
./bin/riffine play --project loop.riffine.json
```

To stream the selection once without rendering it first:

```sh
./bin/riffine preview --project loop.riffine.json
# Run the same worker/device path without sound and print diagnostic JSON:
./bin/riffine preview --project loop.riffine.json --silent
```

`preview` verifies the complete source identity before opening its streaming
worker. It uses fixed saved parameters and previews an enabled loop once.
`--silent` explicitly selects a null output device; normal preview requires
CoreAudio on macOS or WASAPI on Windows and reports device failures.
Pause/seek/live edits, continuous loop scheduling and presentation-clock
acknowledgements remain subsequent work. Preview diagnostics count PCM copied
into callbacks, not hardware presentation or completed practice loops.

Press Ctrl+C to stop playback. An enabled loop is rendered once for `render` and
repeated for `play`. Disable it using `loop --enabled=false` with a new `--out`.
All write commands refuse to replace existing files. Save edits under a new name.
`inspect` emits JSON; add `--verify` to check the source SHA-256 and metadata.
`relink` accepts a moved copy of the exact original audio. Run `help` for flags.

## Current limits

- Import: RIFF/WAVE integer PCM 16/24/32-bit, mono/stereo, 8–192 kHz. No float,
  extensible WAV, RF64, MP3 or FLAC import yet, even though miniaudio supports more.
- Render: PCM16 WAV; independent speed 0.25–2x and pitch ±12 semitones. The chosen
  DSP can introduce artifacts, especially at extreme slowdown and on transients.
- Native offline rendering uses the bounded, latency-trimmed selection kernel
  with fixed PCM blocks. PCM16 WAV output must fit the standard RIFF size limit.
- Each render has a 5 ms edge fade (shorter for tiny selections). Loop playback
  avoids hard edges but is not a crossfaded, sample-seamless live loop engine.
- Waveform data is JSON, not a graphical view. Projects contain metadata and a
  relative source path, not audio. Moving across Windows drives may use an absolute
  reference; use `relink` after moving machines.
- Hardware playback and perceived quality require listening tests on each target.
  Automated tests measure generated tones; they do not establish musical quality.

## Layout

```text
core/audio/       WAV inspection, fingerprinting and waveform peaks
core/project/     Project format, validation, persistence and relinking
core/engine/      DSP renderer and playback interfaces
core/practice/    Host-neutral application operations
adapters/native/ Go adapter for the native helper process
cmd/riffine/      CLI entry point
internal/cli/     Flags and presentation
native/          C ABI, Signalsmith/miniaudio implementation and helper
scripts/         Build and test commands
docs/            Implementation plan, ADRs and validation
web/             React + TypeScript local music-library client
```

See [the implementation plan](docs/implementation-plan.md),
[architecture decision](docs/adr/0001-cli-first-core.md), and
[third-party notices](THIRD_PARTY_NOTICES.md). Contributions: [CONTRIBUTING.md](CONTRIBUTING.md).


Portable session documents now support source-time regions, markers, notes,
named loops and bookmarks. Version-1 documents migrate explicitly on read;
`riffine annotate --project song.json --out annotated.json --annotations notes.json`
saves a new v3 document. [Examples and migration details](docs/validation/portable-sessions.md).

`riffine save --project song.json --speed 0.6 --pitch 3` now saves to the same
project with revision protection and a readable previous-revision recovery copy.
[Saving and recovery](docs/validation/session-revisions.md).


The shared core also supports bounded WAV frame reads and library indexing.
`riffine scan --sources sources.json` emits bounded JSON batches with actual
format capabilities and duplicate/unavailable outcomes; see
[the PCM/library validation and recipe](docs/validation/pcm-library.md).


Project v4 records the source codec and optional source precision; v1–v3 PCM WAV
projects migrate explicitly while keeping their settings, annotations and plans.
See [native codec validation](docs/validation/native-codecs.md) for generated
fixtures, supported operations and compressed seek limits.

See [precise waveform detail](docs/validation/waveform-detail.md).

### Browser practice workspace

Practice keyboard controls use Space for Play/Pause, R for Restart and S for
Stop while focus is in the workspace. **Keyboard shortcuts** lets you remap or
disable them and save preferences locally; typing and native button activation
retain their usual keys. See [foreground keyboard validation](docs/validation/browser-keyboard-controls.md).

The browser practice workspace also supports live Play/Pause, Stop, Restart and
Seek for supported WAV sources. Position and completed A/B loops follow the
shared core's device acknowledgements. Use **Close playback** to return to a
one-pass preview. See [live session binding and its validation limits](docs/validation/browser-live-binding.md).

Build the shared core with `sh scripts/build-browser-practice.sh`, then run
`npm --prefix web ci` and `npm --prefix web run dev`. Open the localhost URL,
choose **Open practice workspace → Try generated demo**, and edit a source-time
phrase, speed/pitch, named loops, notes or markers. Download/reopen projects to
keep edits. The browser adapter currently supports WAV PCM editing/waveforms;
practice audio playback is explicitly unavailable. [Validation and remaining
host work](docs/validation/browser-practice.md).


Browser selection preview can use the shared C++ DSP through an AudioWorklet.
With the existing Emscripten 4.0.14 SDK activated, run
`sh scripts/build-browser-dsp.sh` as well as the browser core build, then open the
practice workspace and choose **Preview selection**. It plays the accepted
selection once at saved speed/pitch; live looping and practice counts remain
unavailable. See [browser audio preview](docs/validation/browser-audio-preview.md)
for run steps, host requirements, cancellation and validation evidence.
