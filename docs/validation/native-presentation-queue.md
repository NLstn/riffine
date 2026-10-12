# Native presentation queue and schedule ledger (partial #20)

The native helper now has reusable queue/mapping components for a future live
device adapter. `PresentationQueue` combines the fixed SPSC PCM queue with a
bounded timeline. Its callback path copies finite worker-produced samples or
zeros, updates lock-free counters, and records backend-frame intervals mapped to
copied PCM ordinals. It does no allocation, waiting, logging, decoding, DSP or Go
calls. Queue samples/channels and timeline entries are allocated at construction.

A distinct monotonic backend callback interval is required. Capacity, overlap or
counter failure gates output before consuming an unrecorded range. The first
timeline fault remains available as a diagnostic. Underrun silence, primed output
and paused gaps never advance presented PCM. The serialized observer accepts a
backend device frame in the negotiated PCM rate; it counts only samples strictly
before that position, preserves partial ranges and ignores backwards observations.
Terminal errors cannot advance presentation. Reset requires all participants and
backend output to be quiescent; setting a failure flag alone is not a close fence.

Pause requests become acknowledged at a callback boundary that gates PCM. Samples
remaining in the queue stay available for resume. The host must still wait for
actual device presentation of the copied tail before suspending/resetting output.
An initial primed queue is already gated, so a host need not wait for a callback
on a device that has never started. Resume preserves ordinal mapping across the
backend-frame gap; source positions and practice counts remain in Go.

`ScheduleLedger` retains at most two ordered submitted ranges. On the control
worker it turns presented PCM ordinals into partial/final acknowledgements with
exact uint64 generation/sequence IDs. Drains are bounded and can release one range
at a time without losing the next completion. Duplicate/stale positions do not
emit duplicate acknowledgements. Beyond-schedule positions fail explicitly; resets
retain increasing schedule identity while restarting output coordinates. The
ledger never interpolates source position or evaluates loop/ladder eligibility.

## Evidence

`native-presentation-queue` CTest covers priming, partial/delayed presentation,
backwards clocks, callback-gated pause, retained PCM resume, paused/underrun gaps,
non-finite producer rejection, overflow/overlap failure before copying, error
without a false presentation fence, quiescent reset, bounded two-range drains,
duplicate/stale acknowledgements and uint64 maximum identity.

An integrated trace uses the actual scheduled FLAC decoder/DSP worker, PCM queue,
timeline and ledger. Two selections with different bounds/speed/pitch generate
ahead of a synthetic device position delayed by 512 frames; exactly two complete
schedule acknowledgements appear only after presentation mapping reaches their
ends. Generated completion and copied output are never substituted for that clock.

A three-thread producer/callback/observer fixture checks exact stereo PCM order
and monotonic presentation through 786432 frames. Its accelerated callback keeps
observer lag below 256 quanta; timeline exhaustion is tested separately. This is
concurrency/mapping evidence, not a device cadence, underrun or deadline budget.
Successful callback calls recorded no C++ new/new[] allocation.

Local macOS arm64/AppleClang21 Release, ASan+UBSan and separate ThreadSanitizer
checks passed. The full eight-test native suite, accelerated 1800-second streaming
soak, native-backed Go integrations, Go tests/vet/gofmt passed. Windows/macOS CI
runs the native test; required Linux ASan+UBSan and macOS ThreadSanitizer steps
cover the presentation path. No physical device or musical listening is attested.

## Remaining host boundary

These components do not open a device or enable native live capability. The next
adapter must supply correctly calibrated device-rate frame positions, ordered
pause/close fences, source ownership and persistent helper IPC. Negotiated PCM and
timeline rates must match; resampling needs an explicit mapping. For example,
[WASAPI GetPosition](https://learn.microsoft.com/en-us/windows/win32/api/audioclient/nf-audioclient-iaudioclock-getposition)
uses clock units that must be related to GetFrequency. Apple's
[AudioQueueGetCurrentTime](https://developer.apple.com/documentation/audiotoolbox/audioqueuegetcurrenttime%28_%3A_%3A_%3A_%3A%29?language=objc)
reports queue-relative sample time; neither API permits replacing presentation
with callback submission or a wall-clock timer. Backend implementation, target
hardware drift/loss/recovery, 30-minute sessions and independent listening remain
roadmap gates.
