# Bounded cached waveform pyramids

Date: 2026-10-11. Partial acceptance evidence for #18, using the WAV PCM port
from #17's indexing/decoder foundation.

The shared `core/waveform` package reads fixed PCM blocks once and builds
per-channel min/max levels. Base cells start at 256 source frames, doubling their
span for very long sources until at most 65,536 base cells remain. Every higher
level combines adjacent extrema, without averaging/clamping or adding silence
to a final partial cell. At most 131,071 peaks per channel are retained (about
2 MiB of stereo peak values). Decoder buffers are independent of recording length.

`Session.OpenWaveform` verifies the original identity and returns an immutable
pyramid handle. Clients retain that handle while navigating. Queries touch no
source resource and return at most 4096 owned cells, with original frame ranges,
source sample rate, requested viewport and content identity. Whole aligned cells
may extend outside the viewport; their coordinates remain explicit so clients
clip the drawing without falsely labelling extrema as exact selection samples.
The original selection boundaries are separate from cache resolution. Base
resolution on extremely long tracks is deliberately coarser; exact sample-level
views would require a separate bounded detail request in a later slice.

Cache keys include the complete original byte fingerprint, source dimensions and
algorithm version. The binary format checks header/body CRCs, exact lengths,
derived counts, finite ordered peaks and consistent aggregate levels before use.
Cache reads are capped at 4 MiB. Missing/corrupt/incompatible caches rebuild;
replacement source bytes select a new key. Interrupted generation is not published.
Optional cache errors remain diagnostics and leave a computed waveform usable.
Hosts provide atomic complete cache publication and stable, interruptible readers.
A changed resource must be reopened; existing immutable handles represent their
original source snapshot.

The filesystem adapter hashes opaque keys into a dedicated cache directory and
stages/syncs before platform rename. Failed or cancelled writes preserve the
prior cache. This is atomic cache visibility, not power-loss storage acceptance.
Caches are disposable generated data and contain peaks rather than source audio.

## CLI reproduction

```sh
sh scripts/build.sh
bin/riffine waveform-window --project song.riffine.json --cache build/peaks \
  --start-frame 12000 --end-frame 36000 --bins 200
```

The first call reports `rebuilt`; subsequent calls report `hit`. Opening still
fingerprints the source to verify identity. Keeping a pyramid open in a client
avoids that startup scan for subsequent view changes. Omitting `--cache` builds
in memory and reports `disabled`. Existing flat `waveform` output remains usable.

## Validation and outstanding work

Generated stereo tone checks verify extrema, source coordinates, partial final
cells, strict output bounds, single-frame viewports, cache equality and defensive
query ownership. A generated 30-minute silent source verifies bounded storage;
non-finite samples are rejected and decoder resources close on failure.
Corrupt headers/bodies, trailing data, replacement identities, interrupted
progress and concurrent filesystem publication are covered. Session tests reject
changed audio and prove view queries don't open resources or change edit sequences.
CLI tests verify rebuild/hit behavior and invalid windows. The common host fixture
includes a bounded waveform window for native/WASM/mobile value comparisons.

This slice supports WAV PCM. #18 remains open alongside #17: compressed decoder
parity, exact high-resolution detail, production browser/mobile cache adapters
and connected long-track client navigation still need implementation. No waveform
or codec client acceptance is inferred from the backend checks alone.
