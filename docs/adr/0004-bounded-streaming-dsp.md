# ADR 0004: Bounded native DSP workers and a native PCM queue

Status: accepted for the processing foundation; transport remains outstanding
in #14. Date: 2026-10-10.

The offline whole-selection renderer makes startup depend on selection length.
Introduce a separate opaque C ABI for the same pinned Signalsmith processing
layer. Its owner serializes create/configure/process/reset/flush/limits/destroy
calls on a worker. Format and maximum block size are fixed at creation; speed
and pitch can change between blocks. All buffers are sized from format/block
limits, never song length. The existing offline ABI and CLI remain supported.

Use interleaved float32 at the C boundary to match native decoder/device buffers.
Deinterleave into preallocated worker storage for Signalsmith. This refines the
planar boundary proposed in ADR 0001; the processing algorithm is unchanged.
Counts are per-channel frames, not samples or bytes. Output counts use cumulative
source frames at each speed plus retained fractional carry, avoiding blockwise
rounding drift. No source/output mapping or practice decisions move into DSP.

Retain a native helper process for the future desktop Go transport binding,
with bounded control IPC for resource handles, commands and acknowledgements.
This preserves pure-Go builds, MSVC support and process cancellation/crash
isolation. Decode, DSP and PCM queues will stay inside that helper: samples must
never cross the Go/UI command bridge. The reusable C ABI can also be linked into
an in-process native host. This slice implements neither the persistent control
IPC nor a Go streaming adapter; their latency/error semantics need validation
when the actual device worker is connected. Mobile/browser gates remain #13.

`PCMQueue` is a preallocated single-producer/single-consumer boundary for a native
worker and device callback. Producer writes fail completely when full. Consumer
reads copy available PCM, zero-fill underruns and return only actual consumed
frames. Construction verifies lock-free atomics. Reads/writes contain no
allocation, locks, logging, decode, DSP, Go or UI calls. Reset/destruction require
both sides to be quiescent. Queue capacity is capped at 262,144 mono/stereo frames;
a host must agree on sample rate and channels and bound callback buffer sizes.

The stream exposes input latency in source frames and output latency in output
frames separately. Raw process output includes startup delay. One terminal flush
returns the tail at the final speed; further processing requires reset. Reset
drops history and fractional carry while preserving format/parameters. No
resampling, automatic latency trim, fades, seek preroll or click-free transition
is promised. Dynamic source/output mapping and output-consumed traversal events
belong to the shared transport, with generation resets (#15).

The native tests verify signal frequency/channel separation, frame counts,
invalid-call nonmutation, parameter changes, flush/reset, concurrent queue order
and a generated 30-minute source processed in bounded blocks. This is accelerated
worker validation, not a 30-minute realtime device session. CoreAudio/WASAPI,
startup/seek/parameter latency, underruns, device loss, cancellation and listening
measurements remain unchecked #14 acceptance criteria. See the
[validation record](../validation/streaming-dsp.md).
