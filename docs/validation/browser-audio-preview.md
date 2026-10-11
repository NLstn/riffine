# Browser audio preview (partial #19/#26)

The practice workspace can preview one accepted source selection at its saved
speed and pitch, using the same Go WAV decoder and C++ selection DSP as the
shared core/native path. Source files and project edits stay local.

## Run and test

With the already installed, pinned Emscripten 4.0.14 SDK activated:

```sh
sh scripts/build-browser-dsp.sh
sh scripts/build-browser-practice.sh
npm --prefix web ci
npm --prefix web run dev
```

Open the localhost URL, choose **Open practice workspace**, then **Try generated
demo**. Set speed to 75%, pitch to +3, and apply an enabled 0.25–0.75 second A/B
selection. **Preview selection** plays that saved range once. On hosts with output timestamps, **Pause preview** holds it and **Resume preview**
continues the retained audio. Cancel stops it; you can edit or preview again. A disabled loop previews the whole original.

The optional processing build copies complete dependency notices alongside its
static assets. Core editing still works without those assets, with preview
explicitly unavailable. The host needs AudioContext, AudioWorklet and
SharedArrayBuffer with cross-origin isolation. Vite dev/preview supply
`Cross-Origin-Opener-Policy: same-origin` and
`Cross-Origin-Embedder-Policy: require-corp`; another local/production host must
supply equivalent isolation headers. These requirements follow the
[worker SharedArrayBuffer documentation](https://developer.mozilla.org/en-US/docs/Web/API/WorkerGlobalScope/crossOriginIsolated).

Preview creates/resumes its AudioContext from the user's click, at the original
source sample rate. Unsupported rates or output failures fail explicitly. No
microphone, audio upload, external processing or network audio source is used.

## Resource, processing and output ownership

The session opens and fingerprints original bytes once before passing a borrowed
reader and inspected metadata to `SourcePreviewer`. The browser adapter opens a
WAV cursor from that verified metadata without another full fingerprint. Saved
JSON lacks decoder offsets and cannot be substituted for inspected metadata.
The decoder closes before the core releases its reader.

Go reads fixed 512-frame blocks on the resource/processing worker. Its callback
feeds shared C++ WASM; it is never called by the AudioWorklet. The worker trims
latency/fades in shared C++, then copies output into a 32768-frame interleaved
shared ring with backpressure. The ring allocation is 64 metadata bytes plus
131072 mono or 262144 stereo PCM bytes, independent of source duration.

The AudioWorklet callback only copies/zeros PCM and updates preallocated atomic
integer fields. It performs no allocation, blocking waits, locking, message
posting, decoding, DSP, logging, Go call or practice-policy decision. Invalid
queue/channel state stops output; starvation emits zeros and records underruns.
An EOF flag terminates after the published tail. Uint32 queue cursors wrap;
submission counts use low/high words and a bounded control-side seqlock. A busy
counter snapshot is retried on the worker, never spun on the audio callback.

Cancellation closes the shared queue and awaits processing completion before
releasing Go callbacks/cursor. Workspace close/fatal worker errors cancel setup,
reject pending requests and close the AudioContext exactly once. The UI disables
edits while a preview runs. Where `getOutputTimestamp` is available, a bounded shared timeline maps each
copied block's AudioWorklet `currentFrame` to its PCM ordinal. The host samples
that device timestamp until the final tracked frame has passed it, then closes
the context. Browsers without timestamps retain a conservative host buffer-drain
delay after callback copying. Neither path emits core transport acknowledgements.
See [browser presentation mapping](browser-presentation-mapping.md) and the
[preview pause boundary](browser-output-boundary.md).

## Evidence

- Six native CTest checks and full Go tests/vet/gofmt pass. Source-preview tests
  verify a single source open, inspected-offset reuse, borrowed-reader close order,
  identity rejection and no project/sequence/playback-event changes.
- Production AudioWorklet logic with a separate Node writer thread consumes
  250013 stereo frames in exact order. Tests cover backpressure, primed silence,
  starvation, finite samples, final partial blocks, corrupt cursors, EOF and
  uint32 cursor/counter wrap. This is a transport shim, not browser hardware.
- The actual browser Go/WASM adapter, immutable-file shim, production C++ WASM,
  preview worker and AudioWorklet logic run together in a simulated Node audio
  pump. The 12000–36000 source-frame selection at 0.75/+3 produces 32000 output
  frames and the expected pitch. Missing output queues fail; cancellation and
  retry preserve session sequence 2 and emit no playback-completion events.
- Facade lifecycle tests cover close before worker readiness, pending request
  rejection, gesture resume before awaiting setup, cancellation/fatal errors
  during setup and exactly one AudioContext close during teardown.
- Actual in-app browser verification imported the generated demo, applied the
  0.25–0.75 source-second selection at 75%/+3, and completed a real AudioWorklet
  preview with 32000 copied frames and zero underruns. A longer 25% preview
  cancelled with the shared `cancelled` error and restored editing controls.
  This confirms browser wiring/callback operation; it does not attest subjective
  listening quality or physical device presentation latency.

The underlying selection-worker comparison also ran on Linux CI: maximum RMS
native/WASM difference was 0.002829, within the established raw-stream 0.02
cross-toolchain tolerance. Counts remain exact; no sample-identical DSP or musical
quality claim is made.

## Remaining work

Browser preview currently accepts integer WAV PCM. Live repeated transport,
seek/pause/recovery and ladder/routine playback are still unavailable; `liveTransport` stays false. Copied frames are source-rate
AudioWorklet submission diagnostics, not device presentation or completed loops.
The [Web Audio specification](https://www.w3.org/TR/webaudio-1.0/#dom-audiocontext-getoutputtimestamp)
separates context/device timestamps; a production live adapter must implement and
validate the complete device lifecycle and schedule mapping instead of forwarding
callback copies.

FLAC/MP3 browser decoding, actual physical listening/latency, long browser audio
sessions, mobile output/lifecycle and production hosting/packaging remain pending.
Node pumps and a completed generated-tone preview do not close those gates.
