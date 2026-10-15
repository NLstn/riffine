# Browser scheduled output adapter (partial #15/#26)

The next live-device dependency connects shared-core schedules to bounded source
reads, the production selection WASM, AudioWorklet output and device-position
acknowledgements. These adapter modules have focused tests separate from one-pass preview. The
[following session-binding slice](browser-live-binding.md) installs the Go
LiveDevice factory and workspace controls; live capability becomes true only
after a successful attachment.

## Worker ownership and routing

`ScheduledSelections` accepts two outstanding traversals, plus one temporary
presentation prefix during [continuous parameter edits](live-parameters.md). Each
schedule supplies its verified source handle/range, speed/pitch, exact output
range and explicit edge fade. C++ checks processing dimensions and applies the
gains; the host does not invent loop/routine policy. Decoding requests absolute
source positions in blocks of at most 512 frames. Output blocks are at most 4096
frames with backpressure through the existing fixed ring. A reset/close waits for
an in-flight source read and destroys pending processors before its resource fence
completes. Repeated close calls share that fence.

A fixed 32-byte shared counter carries the main thread's mapped PCM presentation
position and lifecycle state back to the resource worker. Low/high frame words
are published through a bounded seqlock. Busy snapshots return no new evidence.
Fresh buffers identify each generation; old counters cannot affect its schedules.
A separate bounded ledger routes cumulative presented frames to ordered partial
and complete schedule acknowledgements. Bounded draining retains undelivered
entries. Duplicate device positions produce no duplicate acknowledgement.

Generation and schedule identifiers use canonical decimal strings in the JS port,
preserving every uint64 bit (including values above JS's exact Number range).
Frame coordinates require safe integers; invalid/regressing/future positions fail
without manufacturing evidence. The core continues to own source interpolation,
traversal eligibility and loop counts.

## Main-thread device half

`LiveOutputHost` owns AudioContext, its worklet node, presentation monitor and
callback gate. Reset suspends before releasing old output and creates fresh queue,
timeline and presentation-counter buffers. Start resumes the context and opens a
primed queue. Suspend gates copying, waits for the exact copied boundary to pass
the output timestamp, then suspends and publishes quiescence. A worker processing
failure closes its queue and requests main-thread cleanup; the worker only exposes
that failure to acknowledgement draining after the host publishes its device stop
fence. Invalid queues or stalled presentation trigger explicit host teardown.

The processing worker reports starvation while scheduled PCM remains pending or
before accepting a continuation after a gap. It never repeats the last audio or
advances a loop counter to conceal a missing schedule. Final idle silence can let
the device timestamp catch up; it does not acknowledge any additional PCM.

## Evidence and remaining integration

`node scripts/check-browser-schedules.mjs` runs the actual C++ WASM selections,
production worklet logic and presentation mapper with synthetic Node timestamps:
two 12000–36000 source-frame ranges at 0.75/+3 produce 64000 output frames. A
copied tail remains unacknowledged until the timestamp passes it. Tests cover
partial/complete routing, bounded draining, duplicate positions, exact uint64 IDs,
fresh generations, in-flight reads, failure publication and resource cleanup.

`node scripts/check-browser-live-host.mjs` uses the actual main-host controller,
worklet logic and timestamp monitor with an AudioContext transport shim. It checks
suspend ordering, fresh reset buffers, exactly one context close, failure
publication after the close fence and stalled device-position teardown. Existing
preview, boundary and facade lifecycle tests remain required.

The [session-binding validation](browser-live-binding.md) covers the subsequent
Go factory, main/worker bridge, workspace controls, recovery context rearming and
actual browser repeated-loop trace. This dependency slice does not attest
physical presentation accuracy, independent device latency or listening quality.
