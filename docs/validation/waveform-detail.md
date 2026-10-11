# Exact waveform detail (issue #18)

The existing cached pyramid provides overview windows using complete source-aligned
cells. `waveform.Detail` adds precise extrema inside a requested `[start,end)`
source-frame range. One-frame cells expose that sample's value in both min and max;
coarser detail cells contain only their explicitly reported coordinates. No values
are averaged, clamped or extended beyond the selection.

A session's `OpenWaveformDetail` verifies original identity and owns a resource plus
PCM cursor. Keep the returned handle across gestures, then close it before relinking
or releasing host storage access. Queries do not reopen or fingerprint the source,
change settings, or increment command sequences. They run on an analysis worker;
no decoded samples cross a realtime/UI control boundary.

Each query permits at most 1,048,576 source frames and 4,096 result cells. Scratch
PCM is at most 8,192 stereo frames (64 KiB); result peaks and coordinates are bounded
independently of song length. Wider views use the cached pyramid. WAV and FLAC seek
directly. Current MP3 exact seeking replays a prefix from frame zero to preserve
reservoir/overlap state; memory stays bounded, but seek CPU scales with source
position. This limitation is explicit rather than treating an approximate MP3 seek
as sample-accurate. Codec seek acceleration remains part of #17.

Cancellation is checked before seeking, during bounded reads and before publishing
a result. No partial window is returned. A decoder error or cancellation after
starting decoding marks the detail handle failed; close and reopen it. Invalid
requests and already-cancelled contexts do not touch its decoder. The underlying
source must remain stable for the handle lifetime, as required by resource ports.

CLI example after importing a source:

```sh
riffine waveform-detail --project song.json --start-frame 12345 --end-frame 12348 --bins 4096
```

Checks:

- A synthetic 2^40-frame stereo source queries only three selected frames, without
  allocating or traversing the rest of the source; repeated queries retain the cursor.
- Golden WAV samples match detail values exactly across decoder block boundaries.
- Generated FLAC/MP3 detail values match sequential decoding at MPEG block boundaries,
  arbitrary mid-source positions and the exact final frame, on native CI.
- Stereo, silence and above-full-scale synthetic samples retain their source values.
- Bounds, truncation, non-finite PCM, cancellation and close/resource ownership checks.
- The same three-frame detail result is included in the complete Go/WASM and mobile
  C-to-Go host conformance report.

Client rendering/gestures and production browser/mobile decoder/cache adapters
remain in #19/#26–28. This slice supplies the shared precise-window API, not those
clients or physical-device validation.
