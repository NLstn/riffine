# Native WAV/FLAC/MP3 and codec-aware projects

Date: 2026-10-11. Partial #17, extending #18's waveform prerequisites.

The existing immutable miniaudio layer now exposes an opaque worker-only decoder
C ABI. It opens original files read-only, reports codec/rate/channels/source frames
and optional original precision, reads at most 8192 interleaved float32 frames,
and supports source-frame seek/EOF. Unicode paths use the platform wide-file APIs
on Windows. Decoder, preview and renderer use the same original source coordinates.
No new processing/codec runtime dependency or source revision was introduced.

WAV remains integer PCM 16/24/32-bit; float/extensible WAV is not advertised.
FLAC source precision is reported when available. MP3 precision is absent, not
invented as 16/32 bits from the decoded worker buffer. The pinned decoder honors
available encoder delay/padding metadata. Without that metadata, its emitted
frame sequence is the portable source clock. Exact MP3 seeks replay from frame
zero to preserve bit-reservoir/overlap state: memory is bounded, but seek cost
increases with source position. A future optimized seek table must retain the
same tested values. Metadata inspection may scan compressed frame headers;
full original fingerprinting is streamed separately, including tags.

Preview `decodedBeforeStart` now includes discarded MP3 seek replay as well as
selected input decoded for prefill. Startup timing includes that native seek work
and device initialization, but excludes earlier Go identity verification. Callback
submission remains diagnostic, not presented-output acknowledgement.

The Go native decoder adapter probes the helper's actual versioned format report
before advertising codecs. WAV uses the pure-Go PCM port; compressed resources
require filesystem-owned readers and run an isolated worker. Binary PCM is bounded
and explicitly little-endian. Analysis runs on a worker; realtime preview keeps
all decode/DSP/PCM inside C++. Read contexts can kill a stalled worker, and stream
teardown leaves the borrowed original reader open. Hosts without the new helper
retain WAV capabilities. Extensions never substitute for decoder inspection.

Flat waveform extrema now use the same shared Go PCM algorithm for every codec.
The cached pyramid works through that same decoder port. The CLI import, verify,
relink, scan, waveform/window, render and fixed-parameter preview can use native
codecs; live transport/ladder playback remains unavailable.

## Project migration

Project v4 saves explicit `encoding` and optional original `bits`. Generic
sample rate/channels/frame count and complete original SHA-256 establish source
coordinates and identity. Old v1–v3 schemas are decoded separately, reject codec
fields under old labels and migrate as PCM WAV. Speed ratios, pitch, source-second
loops, named annotations and plan definitions are preserved. Existing source/project
bytes are never rewritten just by opening; saving uses the established exclusive
or revision/recovery contract. Compressed frame counts are not limited by RIFF's
32-bit data-chunk size. Command API version remains 1.

## Evidence and reproduction

Original generated FLAC/MP3 fixtures and encoder provenance live in
`native/testdata`; both decode to 48,000 stereo frames at 48 kHz. Native checks
compare exact sequential/seek samples at 0, 1, 1151, 1152, 12345, near EOF and EOF;
they also check source precision, stereo frequency/energy, Unicode paths,
invalid-call nonmutation and malformed input. Go integration checks binary framing,
identity preservation, seek equality, waveform values, selection render/preview
counts and capability ownership. A controlled stalled test process verifies that
read cancellation kills the worker. Golden project tests preserve v3 plans/units
and reject relabelled codec metadata.

```sh
sh scripts/build.sh --native
sh scripts/test.sh --native
bin/riffine import --audio native/testdata/generated-stereo.mp3 --out build/mp3.json
bin/riffine inspect --project build/mp3.json --verify
bin/riffine waveform-window --project build/mp3.json --cache build/peaks --bins 50
bin/riffine preview --project build/mp3.json --silent
```

Local generated MP3 import/verify/null preview completed with exactly 48,000 copied
frames and zero underruns. This establishes worker/queue conformance, not hardware
presentation or musical listening quality. Native CI must pass on Windows/macOS
before merging. #17 stays open: browser/mobile compressed resource adapters,
platform tag parsing, client capability parity and large-library client integration
remain outstanding. MP3 seek responsiveness and physical playback remain measured
production gates rather than implied by a one-second fixture.

## Retained source identity

Compressed metadata/PCM now use inherited read-only original handles, preserving
identity across pathname replacement and leaving the caller's cursor/reader
untouched. The helper advertises borrowedSources and older helpers require a
rebuild. See [native-borrowed-source](native-borrowed-source.md) for the resource,
cancellation and platform checks. Direct-path preview/render ownership remains
separate work.
