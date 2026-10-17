# Browser audio format support

The music library's **Practice** action and **Choose audio / relink** accept
PCM WAV, MP3, FLAC, M4A (AAC/Apple Lossless) and Ogg Opus. Speed/pitch, A/B looping, seek, waveform detail, named
phrases, saved projects and speed ladders use the existing shared Go session
and C++ processing path. Source coordinates and project hashes refer to the
original file; no converted WAV is created or substituted.

## Try it

Build the browser core and DSP using the README instructions, then open the
localhost app. Choose a music folder and click **Practice** beside a supported track. Set **Speed (%)** and **Pitch (semitones)** and apply them. Set selection
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

Supported decoded dimensions are mono/stereo, 8–192 kHz. M4A must contain AAC
or Apple Lossless; Opus must use the Ogg container (`.opus`, `.ogg`, `.oga`).
Protected media, Ogg Vorbis, WMA and other library-indexed formats remain
unsupported. The native CLI/helper still supports WAV/MP3/FLAC. Browser format
support uses pinned WASM decoders and does not depend on browser codec availability.

### M4A and Opus worker

A pinned libav.js 6.10.9 opus variant (FFmpeg 9.0, libopus 1.6.1) handles Ogg
Opus. Upstream 6.x binaries omit AAC/ALAC, so the explicit `libav-m4a` npm alias
retains the pinned 4.10.6 fat variant (FFmpeg 6.1.1) for M4A compatibility.
`npm run dev`/`build` packages both decoders plus their complete license inventories
and separate corresponding-source trees. The selected decoder loads locally only
when opening M4A or Opus; no audio or decoder code is fetched from a CDN.

The block-reader device borrows the same original File and serves slices of at
most 64 KiB. It reads one compressed packet at a time, decodes at most 8192 frames
per block, and caps its WASM allocator at 64 MiB (imported memory for 4.x;
the checked `getHeapMax` packaging adaptation for 6.x). MP4 indexing also consumes that heap;
files exceeding its capacity fail. An initial bounded-memory decode counts actual
frames without retaining whole-song PCM. AAC delay metadata and the MP4 end
exclude encoder padding; libopus applies pre-skip and the final granule. The
original bytes remain the project fingerprint, and saved v4 projects identify
the codec as `aac`, `alac` or `opus`. ALAC precision is left unspecified by this
adapter; it does not infer source precision from its decoded sample storage.

Backward seeks reopen the demuxer/decoder and replay from zero; forward seeks
continue sequentially. Both preserve the exact decoded sample sequence, including
loop endpoints. Seeking far into a long file can take longer. Cancelling any
operation terminates the owning worker and fences its callbacks.

## Startup and import diagnostics

Browser console messages prefixed with `[riffine:practice` identify each workspace
client and worker. They include startup stages, import/waveform request timings,
file extension/size, and shutdown reasons with error stacks. Routine transport
polling is quiet. Audio contents, full source filenames and saved documents are
not included in these diagnostics.

Startup failures retain their original cause for pending and later requests;
a timeout reports that the core did not load within 30 seconds. Intentional
workspace cleanup is logged separately. React Strict Mode may create and dispose
an initial client before starting its replacement; that expected cleanup does
not fail the replacement's library import.

To investigate a failure, open the browser console before clicking **Practice**,
enable **Preserve log**, and filter for `riffine:practice`. Expand the error and
its context to see the failing request or startup stage.

## Evidence

- `node --experimental-strip-types scripts/check-practice-facade.mjs` verifies
  original fatal/timeout causes survive subsequent requests, error diagnostics
  retain stacks, and successful transport polling does not flood the console.
- `node scripts/check-practice-workspace.mjs` mounts the actual workspace with
  React DOM Strict Mode and checks library auto-import ownership, late disposed
  success/rejection, operation locking, genuine errors and cleanup.
- Local checks decode 31 existing full-length M4A/Opus tracks; actual in-app
  browser library-row imports and complete waveforms pass for a 192-second AAC
  track and a 326-second Opus track. Personal audio is not part of the fixtures
  or CI, and this does not establish every browser's native folder-picker path.
- `node scripts/check-browser-codecs.mjs --media-only` runs in required web CI:
  M4A/AAC, M4A/ALAC and Opus decode to 48,000 frames; reference differences are
  below 0.00004. ALAC reference samples match exactly. This mode needs npm assets
  and the Go browser build, without requiring Emscripten or a native helper.
- New formats also run through the actual live binding with
  `RIFFINE_LIVE_SOURCE=native/testdata/generated-stereo.m4a` or `.opus`; Opus
  additionally exercises saved speed ladders with `RIFFINE_LIVE_PLAN=1`.
- The module packaging regression simulates a browser module worker's
  `importScripts` surface and checks that loading libav preserves the codec
  worker's message handler. Actual in-app browser M4A and Opus import, waveform
  and live A/B looping pass; M4A also runs at 75% speed / -2 semitones.


- `node scripts/check-browser-codecs.mjs build/native/riffine-audio`: generated
  stereo MP3/FLAC/M4A/ALAC/Opus, native or independent FFmpeg reference PCM, exact random seeks/EOF,
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
  transport reported 34 completed loops.

CI builds the actual browser assets and native decoder, requires compressed
conformance plus MP3/FLAC/M4A/Opus live binding, and saves their JSON evidence. These
checks establish wiring and generated-signal behavior, not subjective listening
quality for every music recording or platform.

References: [miniaudio decoding](https://miniaud.io/docs/manual/index.html)
and [Emscripten C++/JavaScript integration](https://emscripten.org/docs/porting/connecting_cpp_and_javascript/Interacting-with-code.html).
