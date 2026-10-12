# Borrowed bounded native render (partial #17/#20/#25)

CLI render/play now render through `engine.SourceRenderer`: the shared session
opens, fingerprints and inspects one reader, lends it to the adapter and closes
it only after native decoder/process teardown. WAV/FLAC/MP3 all use the original
resource rather than reopening its source path. Helpers advertise `borrowedRender`;
older helpers request a rebuild without disabling their decoding capability.
The standalone explicit-path render API remains available.

Both native render entry points now use the existing bounded selection kernel
also used by native preview/scheduled processing and browser processing. The old
whole-selection float buffers and separate offline Signalsmith exact-processing
path are removed. Decode uses fixed source blocks; encoder output uses 4096-frame
blocks. Speed/pitch, latency trim, output-frame count and edge fade follow that
shared processing kernel. Output is quantized/clamped to PCM16 WAV and must fit
standard RIFF size limits. This changes rendered PCM relative to the old offline
algorithm; musical quality still requires listening on target hardware.

The adapter renders into an owned sibling staging directory, validates the WAV
and expected dimensions, syncs/closes it and publishes with an exclusive hard
link. Existing/concurrently created outputs are preserved. Filesystems lacking
hard-link publication fail explicitly. Cancellation kills/waits for the helper,
removes staging files and leaves the original reader open. Native failures remove
only the output they exclusively created; they never delete existing output.
Offline render produces no live playback acknowledgements or practice counts.

## Evidence

- Shared-core ownership test verifies one reader remains open through processing,
  closes afterward, rejects identity mismatch/pre-cancellation and leaves session
  state/sequence/events unchanged.
- Mandatory native-backed Go tests compare rendered bytes through path/borrowed
  APIs for WAV/FLAC/MP3 after replacing Unicode source paths with invalid audio.
  They verify original fingerprint/cursor ownership and no staging leaks.
- Native C++ tests compare PCM16 decoded render samples exactly with quantized
  shared preview kernel samples; existing output is refused. A generated sparse
  30-minute stereo selection renders at 200% while its largest C++ allocation stays
  below 16 MiB, exercising a range rejected by the former whole-buffer limit.
- Controlled stalled native-worker cancellation checks teardown before return and
  unpublished/cleaned output. A concurrent-destination test preserves its bytes.
- Full native CTests, native-backed Go tests/vet/gofmt, Go race checks and local
  ASan+UBSan cover changed paths; target native runtime checks are required in CI.

This exports one selection at fixed settings. Finite routines, rests/clicks,
progress reporting and history remain roadmap work. No listening or device-clock
acceptance is inferred from these generated fixtures.
