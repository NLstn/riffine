# Browser practice editing (issues #19 and #26)

The library now opens an injected `PracticeFacade` workspace. Its default adapter
runs a persistent Go/WASM session bridge on a local worker. Settings, loop and
annotation validation, identity verification, command sequencing, project
migration/encoding, waveform pyramids and precise zoom execute in the same core
as the CLI. React owns form drafts, gestures, labels and drawing only.

## Run and try it

```sh
sh scripts/build-browser-practice.sh
npm --prefix web ci
npm --prefix web run dev
```

Open the displayed localhost URL, choose **Open practice workspace**, then **Try
generated demo**. The demo is produced by the core tone writer. Set speed to 75%,
pitch to +3, and select 0.25–0.75 source seconds. Apply the A/B loop, save a named
phrase and notes, and use exact selection zoom. Download a project to retain the
accepted edits. Reopen it with the matching original WAV; a different original
fails identity verification. User-supplied files and project bytes remain local.
No audio upload or server processing occurs.

You can also open an integer PCM WAV from the existing library or the workspace's
file picker. Library preview controls are disabled while practice is open, so
there is no independent HTMLAudioElement practice state. This adapter currently
advertises WAV PCM, without live practice transport or offline DSP rendering.
The practice play button is disabled and the limitation is visible. FLAC/MP3
practice decoding, live AudioWorklet/DSP, desktop binding, autosave storage and
hand-free transport controls remain in #15/#17/#20/#21/#26–28; these issues are
not closed by this slice.

## Resource and request limits

A selected browser File is immutable and stays in its worker. FileReaderSync
reads at most 64 KiB per chunk; the reader yields between reads so cancellation
messages run. It never makes a whole-source ArrayBuffer or posts decoded PCM to
React. Core waveform work and result limits still apply. The workspace retains
one pyramid/detail cursor across views; replacing a successful session closes
old handles. Failed or cancelled imports leave the existing session intact.

Binding requests use version 1, strict fields and bounded JSON (project limit
plus 4 KiB envelope). Shared commands preserve their independent API version and
successful contiguous sequence. Window/resource/command fields are rejected
outside their corresponding operation. Browser projects are explicitly exported
as new downloads; direct file updates are unavailable in this adapter. Worker
termination releases selected File and Go state ownership.

## Evidence

- Full Go tests/vet/gofmt and native tests; targeted binding/session race checks.
- Native Go and Go/WASM compare complete shared host reports across 20 calls.
- `node scripts/check-browser-practice.mjs` compares 12 complete responses per
  iteration across 20 browser-worker runs, including invalid versions/commands,
  duplicate sequence, precise detail, overview, event draining and export.
- The worker check reopens an exported project and cancels a 32 MiB import,
  confirming the prior session remains. The largest observed read is 32 KiB.
  It uses a controlled FileReaderSync substitute; this is distinct from browser UI evidence.
- Actual in-app browser FileReaderSync imported a generated WAV, displayed its
  overview and exact phrase zoom, rejected +13 semitones, accepted corrected
  75%/+3 edits, retained a named phrase/note, and downloaded a version-4 project.
  Inspection of that download confirmed the settings, source codec, region and
  named-loop reference. The demo path uses the same real browser File port.
- TypeScript and production Vite build pass. Browser binding conformance is part
  of the web CI job; mobile conformance includes the same bridge trace.

Physical audio, complete keyboard/screen-reader workflows, automatic persistent
cache storage, client production codecs and browser/mobile transport are still
outstanding roadmap work. No audio-presentation acknowledgement is synthesized
from UI timers, waveform queries or browser library preview events.
