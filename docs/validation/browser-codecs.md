# Browser MP3 and FLAC practice

The music library's **Practice** action and **Choose audio / relink** accept
PCM WAV, MP3 and FLAC. Speed/pitch, A/B looping, seek, waveform detail, named
phrases, saved projects and speed ladders use the existing shared Go session
and C++ processing path. Source coordinates and project hashes refer to the
original file; no converted WAV is created or substituted.

## Try it

Build the browser core and DSP using the README instructions, then open the
localhost app. Choose a music folder and click **Practice** beside an MP3 or
FLAC. Set **Speed (%)** and **Pitch (semitones)** and apply them. Set selection
start/end in source seconds, apply the A/B loop and press **Play**. Download a
project to retain edits; reopen it with the same original audio. The generated
MP3 demo also exercises the real browser File import path without selecting
personal music.

## Host adapter

The same pinned miniaudio revision used by native decodes compressed files in a
separate, cancellable worker. Its custom read callbacks use FileReaderSync on
immutable selected File slices of at most 64 KiB. The WASM heap stays fixed at
32 MiB per decoder worker; PCM requests hold at most 8192 mono/stereo frames.
The worklet only copies the existing bounded output queue. No whole compressed
file ArrayBuffer or whole-song PCM buffer is created by the browser adapter.

The Go decoder implements the existing Decoder/PCMDecoder ports, fingerprints
all original bytes including tags, and uses the shared waveform algorithms.
FLAC source precision is preserved; lossy MP3 has no fixed PCM precision.
Sequential MP3 reads avoid redundant seeks. Random MP3 seeks rewind and decode
forward in bounded blocks, matching native reservoir/offset behavior. This can
make seeking far into a long MP3 slower; cancelling an open, read or seek
terminates its worker and fences callbacks before releasing borrowed resources.
Unsupported/corrupt files return an error; previously opened projects survive
failed or cancelled replacement opens. WAV editing remains available when
optional processing assets are missing.

Supported decoded dimensions are mono/stereo, 8–192 kHz. M4A/AAC, OGG/Opus,
WMA and other library-indexed formats do not yet support practice. Format
support uses the pinned decoder rather than browser-dependent decodeAudioData
or HTML audio playback-rate substitutions.

## Evidence

- `node scripts/check-browser-codecs.mjs build/native/riffine-audio`: generated
  stereo MP3/FLAC, native versus WASM dimensions/PCM, exact random seeks/EOF,
  full overview and three-frame detail, original SHA-256, saved speed/pitch/loop
  reopening, malformed input and cancelled-open lifetime fences. FLAC PCM is
  identical; the native/WASM MP3 difference is at most 1/32768 in this fixture
  (scalar/SIMD decoder rounding; tolerance <0.00004).
- `RIFFINE_LIVE_SOURCE=native/testdata/generated-stereo.mp3 node
  scripts/check-browser-live-binding.mjs` (also FLAC): real Go/WASM and C++/WASM,
  stereo output, completed loops, live edits, pause/seek/resume, stop, recovery
  and teardown; its device timestamps are synthetic.
- MP3 saved speed-ladder mode also passes with `RIFFINE_LIVE_PLAN=1`.
- Local native build plus `sh scripts/test.sh --native`: all ten CTests and
  native integration Go tests pass. Go tests/vet, web build, facade checks,
  WAV binding and 100-case selection/rate conformance remain passing.
- Actual in-app browser: real generated MP3 File, waveform, 75% speed,
  -2 semitones, 0.2–0.8 second A/B loop, Play and Pause; the device-backed
  transport reported 34 completed loops. Automated folder-picker access was
  unavailable in this browser; folder indexing is unchanged and the row
  Practice action passes the same selected File to the verified import path.

CI builds the actual browser assets and native decoder, requires compressed
conformance plus MP3/FLAC live binding, and saves their JSON evidence. These
checks establish wiring and generated-signal behavior, not subjective listening
quality for every music recording or platform.

References: [miniaudio decoding](https://miniaud.io/docs/manual/index.html)
and [Emscripten C++/JavaScript integration](https://emscripten.org/docs/porting/connecting_cpp_and_javascript/Interacting-with-code.html).
