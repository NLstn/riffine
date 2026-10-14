# Riffine

A local-first, open-source guitar practice engine. **Working name; pre-alpha.**

Riffine is a local-first guitar practice app. The repository contains a Go
library and CLI plus an early React, TypeScript and Vite music-library client.
The client lets a user choose a local music folder and reads common embedded
tags in the browser. Audio files are not uploaded. The Go CLI imports PCM WAV and, with the tested native helper, FLAC/MP3
songs, saves versioned practice projects, sets independent speed/pitch and A/B
loops, extracts waveform peaks, renders practice audio through Signalsmith
Stretch, and plays it through miniaudio.

The browser client indexes local music and provides a practice workspace for
PCM WAV, MP3, FLAC, M4A (AAC/Apple Lossless) and Ogg Opus with independent speed/pitch, live A/B looping, waveform
navigation, named phrases and saved projects. The current CLI playback renders the selection to a temporary
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

- Browser practice import: RIFF/WAVE integer PCM 16/24/32-bit, MP3, FLAC,
  M4A containing AAC or Apple Lossless, and Ogg Opus (`.opus`, `.ogg`, `.oga`),
  mono/stereo, 8–192 kHz. Native CLI/helper support remains WAV/MP3/FLAC.
  Protected M4A, Vorbis, float/extensible WAV, RF64 and other library-indexed
  formats do not yet support practice.
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

See [the maintained delivery roadmap](docs/roadmap.md),
[the implementation plan](docs/implementation-plan.md),
[architecture decision](docs/adr/0001-cli-first-core.md), and
[third-party notices](THIRD_PARTY_NOTICES.md). Contributions: [CONTRIBUTING.md](CONTRIBUTING.md).


Portable session documents now support source-time regions, markers, notes,
named loops and bookmarks. Version-1 documents migrate explicitly on read;
`riffine annotate --project song.json --out annotated.json --annotations notes.json`
saves a new v5 document. [Examples and migration details](docs/validation/portable-sessions.md).

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

Build the actual shared core and processing assets (the DSP build requires an
already activated Emscripten 4.0.14 SDK):

```sh
sh scripts/build-browser-practice.sh
sh scripts/build-browser-dsp.sh
npm --prefix web ci
npm --prefix web run dev
```

Open the localhost URL to start in **Practice**. Choose **Open song** for a
WAV, MP3, FLAC, M4A or Opus file, or open **Music library → Choose music folder**
and click a song. It opens directly in the large waveform player.

Press **Play** and adjust **Speed** or **Pitch** with the sliders, numeric fields,
speed presets or pitch step buttons. Changes take effect during playback without
an Apply button. Drag across the waveform to select an A/B loop; drag its handles
or click for A and Shift-click for B to refine it. **Loop** toggles repetition,
**Zoom to loop** enlarges the selected phrase, and the seek bar navigates the song.
The playhead and repeat count follow the shared core's device acknowledgements.

**Save project** adds a copy to the visible **Training projects** sidebar. Reopen
it to update that saved copy with revision protection. Projects retain speed,
pitch, loops, phrases, notes and training plans. The matching original is reused
when available in the current library; otherwise choose it when prompted. Audio
stays on your device and is verified before a saved project opens. Project options
include previous-version recovery and portable download.

Use **Try a demo song** for a generated audio fixture. Saved phrases sit below the
player; notes, markers, training plans, timing and keyboard preferences are in
expandable panels. The layout adapts to smaller screens.

`npm run dev` and `npm run build` also package the pinned M4A/Opus WASM
decoder and its corresponding sources locally. It runs in a cancellable worker;
audio stays on this device. `node scripts/check-browser-codecs.mjs --media-only`
checks the new formats without requiring the native helper or DSP build.
Download/reopen projects with the same original source to keep parameters,
named phrases, markers, notes and speed ladders. Audio stays on the device;
no upload, external server or manual conversion is required.

Space toggles Play/Pause, R restarts and S stops while focus is in the workspace.
**Keyboard shortcuts** lets you remap or disable them and save preferences
locally; typing and native button activation retain their usual keys.

[Browser format support and validation](docs/validation/browser-codecs.md),
[live binding](docs/validation/browser-live-binding.md),
[keyboard controls](docs/validation/browser-keyboard-controls.md) and
[selection preview](docs/validation/browser-audio-preview.md).

The browser now supports **Save on this device**, reopening verified originals,
revision-protected saves and previous-version recovery. Production builds cache
the UI and complete processing engines for offline use; wait for the offline-ready
message before disconnecting. Generated core/DSP assets are required for a
production build. See [device saves and offline checks](docs/validation/browser-persistence-offline.md).

Waveform A/B handles support dragging and source-frame keyboard adjustments.
Saved phrases can be updated/removed, and saved markers are visible/editable through
the shared commands. See [selection controls](docs/validation/browser-selection-controls.md).

Saved speed ladders now support count-in, fixed-second rest gaps and a browser
metronome under **Preparation and timing**. BPM means the recording’s original
tempo; click spacing follows song speed, while rest seconds stay fixed.
Project v5 preserves these options and migrates v1–v4 explicitly.
[Timing workflow, validation and host limits](docs/validation/practice-cues.md).
