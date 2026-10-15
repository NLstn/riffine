# Song loading: before and after

Measured on 2026-10-15. The baseline is the runtime merged in PR #75 (`fe6e966`); the optimized run includes the loading changes in this branch. Three fresh session workers per fixture, using the same original generated stereo 48 kHz signals in both runs.

## Results

Ready includes worker startup, import and initial waveform generation. First play is additional time after Ready, ending when the production transport command accepts playing state. Values are medians; times below are seconds.

### 30-second tracks

| Format | Import before → after | Waveform before → after | Ready before → after | First play before → after |
| --- | ---: | ---: | ---: | ---: |
| WAV | 1.255 → 0.060 | 3.915 → 0.079 | 5.255 → 0.223 | 1.411 → 0.031 |
| MP3 | 0.147 → 0.043 | 0.479 → 0.132 | 0.728 → 0.262 | 0.242 → 0.057 |
| FLAC | 0.176 → 0.061 | 0.501 → 0.103 | 0.752 → 0.222 | 0.239 → 0.058 |
| M4A | 0.223 → 0.095 | 0.701 → 0.190 | 1.008 → 0.340 | 0.395 → 0.082 |
| OPUS | 0.277 → 0.144 | 0.849 → 0.209 | 1.174 → 0.394 | 0.421 → 0.089 |

### 180-second tracks

| Format | Import before → after | Waveform before → after | Ready before → after | First play before → after |
| --- | ---: | ---: | ---: | ---: |
| WAV | 6.413 → 0.438 | 19.790 → 0.648 | 26.204 → 1.156 | 6.641 → 0.038 |
| MP3 | 0.773 → 0.061 | 1.943 → 0.457 | 2.758 → 0.590 | 0.743 → 0.082 |
| FLAC | 0.824 → 0.064 | 2.150 → 0.511 | 3.023 → 0.655 | 0.838 → 0.081 |
| M4A | 1.069 → 0.320 | 2.915 → 0.565 | 3.999 → 0.969 | 1.352 → 0.049 |
| OPUS | 1.376 → 0.577 | 4.223 → 0.818 | 5.670 → 1.466 | 1.985 → 0.047 |

For the three-minute WAV, Ready is **22.7× faster** (26.20 s → 1.16 s), and first-play setup is **176× faster** (6.64 s → 38 ms). Initial device-timestamp presentation falls from 6.68 s to 115 ms. M4A first-play setup falls from 1.35 s to 49 ms; Opus from 1.98 s to 47 ms. Presentation counters are browser/device timestamps, not acoustic speaker measurements. There were zero output underrun frames in all 30 optimized sessions and nine uninstrumented confirmation sessions.

## Changes and measured work

1. **Yield after a work budget, instead of before each read.** The resource worker checks elapsed time between bounded reads and yields after roughly 8 ms of work. Reads remain capped at 64 KiB. This retains cooperative cancellation without a browser timer for every small fingerprint/PCM chunk. The baseline requested 1 ms timers but observed approximately 4.8–4.9 ms per timer on long files.
2. **Retain a successful inspection for the selected immutable File.** Original-byte SHA-256 and inspected dimensions/decoder offsets are retained only by that File’s browser resource owner. Reopening readers for waveform/playback reuses the inspection. A new source selection creates a separate owner, including when its filename and size match. Failed/cancelled inspections are not retained. The core still compares inspected identity with saved project identity. Mutable native resources continue to be inspected on each verification.
3. **Pass inspected metadata to waveform PCM.** The core uses the optional verified-PCM decoder capability, avoiding a second inspection. The session passes the current in-memory inspection, including WAV offsets; portable project metadata cannot substitute for it. Decoders without that capability retain their existing path.
4. **Retain exact media duration metadata per File.** A private WeakMap in the codec port stores only metadata successfully measured from the exact File object. New M4A/Opus cursors can skip a repeated full decode to count frames. The initial scan still applies codec delay/padding rules; later PCM reads validate decoded dimensions and retain exact seek behavior. Saved project metadata never populates this cache. No whole-song PCM buffer is retained.

Measured three-minute WAV work:

| Stage | Original bytes read through resource port, before → after | Short timer count, before → after | Timer wait, before → after |
| --- | ---: | ---: | ---: |
| Import | 34.56 MB → 34.56 MB | 1061 → 34 | 5.19 s → 0.148 s |
| Waveform | 103.68 MB → 34.56 MB | 3177 → 50 | 15.56 s → 0.228 s |
| First play | 34.60 MB → 43 KB | 1081 → 1 | 5.24 s → 0.001 s |

Compressed decoder opens fall from **1 / 3 / 2** during import / waveform / first play to **1 / 1 / 1**. Only the import fingerprints the selected File. FLAC cursor creation additionally reads a 26-byte source precision header. M4A/Opus perform one full duration scan across these stages, instead of six. The waveform still decodes the full song once. Median M4A first-play decoder-open time falls from 598 ms to 25 ms, and Opus from 1076 ms to 17 ms.

## Uninstrumented confirmation

The production worker, without profiling hooks, repeated three runs each for the three-minute WAV, M4A and Opus. These medians confirm the improvements:

| Format | Import | Waveform | Ready | First play | First presented |
| --- | ---: | ---: | ---: | ---: | ---: |
| WAV | 0.451 s | 0.662 s | 1.155 s | 0.037 s | 0.117 s |
| M4A | 0.319 s | 0.566 s | 0.961 s | 0.046 s | 0.116 s |
| OPUS | 0.571 s | 0.821 s | 1.449 s | 0.046 s | 0.109 s |

The baseline also had an uninstrumented confirmation: three-minute WAV import / waveform / first play were 6.37 / 20.42 / 6.64 s; M4A was 1.04 / 3.00 / 1.40 s.

## Correctness checks

- All Go package tests and `go vet` pass, including waveform cancellation/cache tests and mutable-source identity verification. A regression test requires waveform generation to use the current inspection with valid decoder offsets.
- Browser Go/WASM adapter conformance (with a controlled Node FileReaderSync) passes, including cancellation during a large WAV import preserving the previous session and reads bounded to 64 KiB.
- MP3, FLAC, AAC, Apple Lossless and Opus checks pass for original-byte SHA-256, reference PCM, sample-identical full output and backwards seeking from a reused inspection, no repeated waveform fingerprint, one waveform decoder open, rejection of same-name/same-size changed files, cancelled opens preserving the previous session, and fenced decoder cancellation.
- Saved-project checks pass for identity mismatch, recovery and revision conflicts. Continuous live pitch/speed updates pass with the same generation/worklet and zero underruns.
- Production web build passes.

## Method and limits

The development page calls actual `readTrack`, `BrowserPractice.open`, initial/cached waveform queries, first `transport(play)`, pause and resume. It uses the production Go/WASM, C++/WASM, codec workers, worklet and timestamp monitor. The localhost-only server prepends counters to the resource worker; production worker files are not rewritten. Counters cover resource-reader bytes/time, short timers and codec RPCs. Resource-byte counters exclude decoder-child-worker compressed reads. Wall times include interop, processing and scheduling.

Every run creates a fresh session worker. Browser/module and OS caches are not forcibly cleared. Local fixture delivery and File construction happen before timing. These are desktop local-development measurements, not a cold-install, disk or React paint benchmark. Metadata parsing and worker startup are separate. Generated tones compress unusually well with FLAC; the 180-second WAV / MP3 / FLAC / M4A / Opus sizes are 34.56 / 4.32 / 4.65 / 4.36 / 5.21 MB. Artwork, cloud sources, mobile devices, folder-indexing concurrency and long-session memory were not profiled.

Initial waveform generation still decodes the complete source and the workspace still waits for it. Persistent waveform caching and changing when the overview appears remain separate opportunities; this change removes the measured repeated work. Cached waveform queries already perform zero source reads/decoder opens.

Full baseline and optimized records, per-stage counters, medians, source hashes and uninstrumented confirmations are in [song-loading-profile.json](song-loading-profile.json). Generated audio and transient output remain in ignored `build/import-profile`.

## Reproduce

With PyAV 16.1.0 and NumPy available to Python, and the normal browser assets built:

```sh
python scripts/generate-loading-fixtures.py
node scripts/serve-loading-profile.mjs
```

Open `http://127.0.0.1:5175/profile-song-loading.html` and press **Profile import and first playback**. It runs 30 cases and saves `build/import-profile/browser-results.json`. Query parameters can select `seconds=180`, `formats=wav,m4a,opus`, or `runs=3`. For uninstrumented confirmation, restart with `RIFFINE_LOADING_PROFILE_PLAIN=1`; results save separately as `browser-plain-results.json`. Preserve earlier measurements before rerunning.
