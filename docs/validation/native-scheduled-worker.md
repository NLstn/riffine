# Bounded native scheduled decode/DSP worker (partial #20)

`riffine_scheduled.h` exposes an experimental serialized worker C ABI. It opens
an immutable UTF-8 WAV/FLAC/MP3 path read-only and retains two decoder cursors.
The host submits Go-compiled schedules with generation/sequence, source range,
output coordinates, speed, pitch and exact edge-fade length. No loop or ladder
policy is implemented in C++; a missing schedule returns zero frames and never
repeats previously generated audio.

Reset requires a strictly newer generation and caller quiescence, releases old
processing cursors and resets output coordinates to zero. Schedule sequence IDs
remain increasing across generations. At most two selections are pending. All
schema/range/capacity validation precedes decoder mutation; invalid requests leave
pending work intact. Decoder/processing runtime failures poison the worker and
require recreation. Result metadata stays zero on errors, even if the caller's
PCM buffer was partly touched. No exception crosses the C boundary.

Each slot borrows a fixed 1024-float source scratch buffer, reads at most 512
source frames and drains at most 8192 output frames per call. A block belongs to
one schedule and identifies its generation, sequence and output start. Processing
uses the existing latency-trimmed selection kernel and explicit scheduled fades.
Output and source sample rates currently must agree. All create/seek/decode/DSP/
destroy work runs off the device callback; no Go callback or UI sample bridge is
introduced. MP3 retains the decoder's bounded-memory prefix replay for exact seek.

`generated_complete` describes processed output only. It is not presentation,
and must never complete a core traversal. The future native host still needs a
bounded PCM queue, schedule ledger, device-rate clock/latency mapping, persistent
control IPC, verified resource ownership and an ordered shutdown fence. Opening
two cursors compares dimensions; it does not replace source SHA verification.
No production native live capability or desktop shell is enabled by this worker.

## Evidence

The new `bounded-scheduled-worker` CTest uses generated stereo WAV and the checked-in
licensed generated FLAC/MP3 fixtures under Unicode paths. It checks two pending
slots, per-block schedule coordinates, exact output lengths, stale/duplicate IDs,
invalid capacity/length/fade rejection, no unscheduled repetition, one-frame/odd
output, 100 resets per codec and replacement of a partially generated selection.
Profiles at 0.25x/-12, 0.75x/+3, 1x/0 and 2x/+12 match the existing bounded preview
kernel sample-for-sample. This is kernel/resource evidence, not musical or device
presentation evidence. Successful reads recorded no C++ new/new[] allocation;
allocation/deallocation and all processing are nevertheless worker-only.

The existing C-compiled ABI test now calls each new exported operation with invalid
inputs and checks ownership/zero-result handling. On local macOS arm64/AppleClang21,
the full seven-test native suite (including the accelerated 1800-second streaming
soak), native-backed Go integrations, Go tests/vet/gofmt passed. The scheduled
worker and C ABI also passed AddressSanitizer + UndefinedBehaviorSanitizer in a
separate Debug build. Windows/macOS CI runs the full native suite; Linux sanitizer
CI checks the new worker/ABI. No thread-race, physical device, real-time deadline,
independent listening or Windows runtime sanitizer measurement is claimed here.
