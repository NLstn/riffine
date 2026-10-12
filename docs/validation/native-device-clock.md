# Native timestamped output boundary (partial #20)

The native helper now has a CoreAudio HAL output backend. It queries the default
output stream without changing hardware settings, accepts native float32 mono or
stereo at a matching source/device rate, and copies bounded native queue PCM in
its IOProc. It performs no allocation, decode/DSP, Go calls, locks, waits or logging
on that callback. Mono may map to both device channels; unsupported formats,
multiple streams and differing rates fail explicitly pending resampling support.

Callback output timestamps map PCM onto device sample-frame coordinates. Device
and stream latency are summed, delaying the presentation boundary; the control
observer uses AudioDeviceGetCurrentTime instead of a timer/submitted count. The
portable clock mapper handles signed native epochs, callback gaps and a fresh
native epoch after a fully fenced suspend. It rejects nonfinite/imprecise,
backwards/overlapping or oversized callback timestamps and bounded-latency errors.

Start waits for an actual timestamped callback. Stop gates callbacks, retires its
IOProc registration and waits for active callbacks; resume creates a fresh IOProc.
The controller must first gate PCM and observe the presented copied tail before
stopping. Queue reset requires quiescence. Device/default-route, format, rate,
buffer and latency notifications fail closed. Destruction retires listeners and
callbacks before freeing state; an unreleasable platform callback terminates the
isolated helper rather than freeing callback-owned memory prematurely.

`riffine-audio device-info` reads output rate/channel/buffer/latency metadata without
starting playback. `build/native/riffine-output-backend-smoke` is an explicit local
hardware diagnostic: it queues only zero-valued PCM and performs three cycles of
start, timestamped presentation, callback gating, tail drain and suspend. Headless
CI compiles this diagnostic but does not claim hardware execution.

## Evidence and limits

The local Mac probe reported 44.1 kHz stereo, 512-frame buffers and 695 frames of
reported device+stream latency. The real silent diagnostic completed three
start/suspend cycles and presented 4608 silent frames. This checks backend clock
and pause/resume wiring; it does not establish musical quality or device-loss,
Bluetooth/aggregate-device, latency measurement or 30-minute acceptance.

The portable clock tests cover latency-compensated partial/final positions,
retained PCM across a native counter restart, signed epochs, invalid/backwards
coordinates, reset and callback allocation checks. A concurrent publisher/observer
fixture exercises clock anchor publication. Full native/Go checks and local
ASan+UBSan/TSan cover the slice; required Linux sanitizers and macOS TSan now include
the clock fixture. Native Windows CI builds the explicit unsupported backend stub.

The backend follows Apple's [IOProc timestamp contract](https://developer.apple.com/documentation/coreaudio/audiodeviceioproc?language=objc)
and [device clock API](https://developer.apple.com/documentation/coreaudio/audiodevicegetcurrenttime%28_%3A_%3A%29?language=objc).
The SDK documents device and stream latency as additive. `AudioDeviceStop` and
IOProc destruction retire the registration; additional active-call guards protect
owned memory during teardown.

This backend is not yet wired into the persistent live control protocol or Go
LiveDeviceFactory, so this slice does not advertise native live capability. That
integration, output-rate conversion, WASAPI, shell/file dialogs and physical
practice/listening acceptance remain #20 work.
