# Browser preview pause boundary (partial #15/#19/#26)

Timestamp-capable preview hosts now offer **Pause preview** and **Resume preview**
while a saved selection plays once. Preview remains distinct from repeated core
transport: pausing does not change project settings, session sequence, loop counts
or practice activity. Cancel/close still end the preview. Hosts lacking output
timestamps keep preview with cancellation and do not advertise pause controls.

## Boundary protocol

The control thread changes running output to `pausing`. A callback may already
be copying one quantum; its counters and presentation entry are published before
the next callback acknowledges `paused`. Paused callbacks output silence and
consume no PCM. They keep the graph running until the device timestamp passes the
exact cumulative copied-frame boundary. Only then does the host call and await
AudioContext.suspend. Its promise is hardware handoff, not independently sufficient
presentation evidence; see the [Web Audio suspend specification](https://www.w3.org/TR/webaudio-1.0/#dom-audiocontext-suspend).

Queued PCM stays in the bounded ring. The worker may prefill remaining capacity
while paused, then waits with backpressure. Pause states suppress the active-output
10-second no-copy deadline; cancellation remains available. Resume calls
AudioContext.resume immediately from the user's action, waits for successful
resume, then opens the queue gate. It does not reset DSP, decode again or discard
retained PCM. Device/context gaps remain excluded from PCM ordinals.

Operations serialize on the host boundary controller. A callback-gate wait and
presentation drain have bounded failure deadlines. Completing the finite preview
wins a racing pause request. Cancellation closes the queue, rejects pending gate
waits, and retains the core reader/callback teardown fence before final context
closure. A boundary failure triggers preview cancellation; this preview controller
is not yet a production implementation of the shared LiveDevice interface.

## Evidence

- `node scripts/check-browser-boundary.mjs`: production queue/consumer, timestamp
  reader/monitor and boundary controller verify suspend ordering, silence while
  paused, retained PCM order, repeated pause/resume, excluded gaps, gesture resume,
  completion races and cancelling a pending gate. Its device clock is synthetic.
- `node --experimental-strip-types scripts/check-practice-facade.mjs`: actual
  facade controls, pause/resume and cancellation retain the core teardown fence
  and close the context exactly once.
- `node scripts/check-browser-preview.mjs`: actual Go/WASM and C++/WASM preview
  stays paused for more than 11 seconds (past the active-output stall deadline),
  resumes, produces exactly 32000 frames, and preserves sequence 2 with no core
  playback-completion events. Cancellation and retry continue to pass.

Actual in-app browser verification paused the whole generated 3-second WAV at
25% speed, retained its paused state while other checks ran, then resumed and
completed it. This attests browser wiring and lifecycle, not independent physical
device accuracy or subjective listening quality.

Live repeated transport, source-position/loop acknowledgements, seek/recovery,
ladder/routine playback, physical-device latency/listening and long browser sessions
remain separate unfinished gates. This protocol supplies the output boundary
mechanism needed for that next adapter work.
