# Original generated codec fixtures

These files contain one second of generated 48 kHz stereo 440/660 Hz signals
with 5 ms attack/release gates. No commercial recording, personal audio or
third-party sound asset is used. They are covered by this repository's MIT license.
The complete original byte hashes/sizes are recorded in `generated-codecs.json`.

`scripts/generate-codec-fixtures.py` records the generator and encoder version.
It uses development-only PyAV 16.1.0 (FFmpeg FLAC/libmp3lame encoders) and NumPy.
Encoder tools/wheels/binaries are not included in Riffine or its runtime packages.
These committed bytes are the conformance fixtures; rebuilding with a different
encoder/toolchain may change compressed identity, even with identical source PCM.

Both decode to 48,000 source frames using the pinned miniaudio implementation.
The MP3 includes available gapless delay/padding metadata. Files without that
metadata use the pinned decoder's full emitted source-frame coordinates; no
client invents encoder-delay offsets. Seeking is compared against the same
sequential decoded stream, including MPEG frame boundaries and EOF.
