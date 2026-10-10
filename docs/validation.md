# Validation record

Date: 2026-10-10. Local environment: Windows x64, Go 1.27.2, MSVC 19.51,
Release native build. CI additionally targets Go 1.24 on Linux/Windows/macOS
and native rendering on Windows/macOS. See the repository Actions page for
remote results; local results do not establish macOS hardware support.

## Completed locally

- Full `scripts/build.ps1 -Native` and `scripts/test.ps1 -Native` passed.
- Go unit/CLI tests, `go vet` and formatting checks passed.
- Generated-tone CLI workflow: import, edit speed/pitch, loop, inspect/verify,
  waveform JSON, render and native playback completed successfully.
- One-second miniaudio playback opened the Windows device and reached the end.
  This verifies the device path, not subjective sound quality.
- UTF-8 input/output filenames, mono/stereo rendering, short selections,
  cancellation and output collision checks passed.
- WAV parser tests cover truncation, invalid format/alignment/byte rate/chunk
  bounds, odd metadata padding, and PCM16/24/32 sign conversion.
- Project tests cover relocation/relinking, fingerprint mismatch, invalid settings,
  unsupported versions, malformed documents and overwrite prevention.

## DSP measurements

Input: generated 440 Hz sine, 48 kHz, amplitude 0.25; source selection two seconds.
Frequency estimated from positive zero crossings in the middle 60% of output.
Allowed frequency error is 2%; duration must match the rounded sample count.

| Speed | Pitch | Expected duration | Measured frequency | Result |
| --- | --- | --- | --- | --- |
| 0.5x | 0 | 4 seconds | 440.0 Hz | Pass |
| 1x | +12 | 2 seconds | 880.0 Hz, both channels | Pass |
| 0.75x | -12 | 2.666667 seconds | 221.2 Hz | Pass |
| 2x | 0 | 1 second | 440.0 Hz, both channels | Pass |

Measured RMS values were approximately 0.172–0.178. These tests check gross pitch,
duration and output-energy behavior. They do not measure harmonic distortion,
stereo phase coherence, perceptual quality, real-time latency or underruns.

## Remaining acceptance work

Musical listening corpus (guitar chords, transients, full mixes), macOS device
playback, live transport and continuous streaming belong to the next milestone.
The current render-first workflow is explicitly a pre-alpha limitation.
