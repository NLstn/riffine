# Riffine

A local-first, open-source guitar practice engine. **Working name; pre-alpha.**

This first implementation is a Go library plus CLI. It imports PCM WAV songs,
saves versioned practice projects, sets independent speed/pitch and A/B loops,
extracts waveform peaks, renders practice audio through Signalsmith Stretch,
and plays it through miniaudio. There is no server, account or audio upload.

Playback renders the selection to a temporary WAV first. Parameters are not
changed interactively during playback yet. Browser, Wails and WinUI clients are
deliberately absent from this phase.

## Build

The core/CLI needs Go 1.24+ and has no external Go dependencies:

```sh
go test ./...
go build -o bin/riffine ./cmd/riffine
```

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
- Native offline processing caps its main input/output float buffers at 256 MiB;
  select a shorter loop for large tracks. Total process memory is higher.
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
```

See [the implementation plan](docs/implementation-plan.md),
[architecture decision](docs/adr/0001-cli-first-core.md), and
[third-party notices](THIRD_PARTY_NOTICES.md). Contributions: [CONTRIBUTING.md](CONTRIBUTING.md).
