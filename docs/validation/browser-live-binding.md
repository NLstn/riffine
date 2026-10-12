# Browser live session binding

The practice workspace can now attach a presentation-aware browser device to the
actual Go session. Play, Pause, Stop, Restart, Seek and speed/pitch/A/B changes
route through the versioned shared-core binding. React polls snapshots every
50 ms while attached; it does not advance position, count loops, or repeat audio.
The saved project contains settings and annotations, not runtime counts.

## Source and device ownership

`adapters/browser.LiveFactory` borrows the original reader already inspected and
fingerprinted by `Session.AttachTransport`. A WAV PCM cursor seeks absolute
source frames and returns at most 512 frames to the resource worker. Go sample
and byte scratch space is fixed (1024 floats and 4096 bytes); JS transfers are
bounded to the requested block. The resource worker processes the same C++
selection kernel as native preview and retains at most two scheduled selections.
Generation/schedule identifiers cross JS as decimal strings, preserving uint64
values; acknowledgement positions must fit the safe JS integer range.

The main thread resumes AudioContext synchronously from the Play user action,
then installs the worklet and live host. The callback only copies bounded PCM,
records its presentation mapping, and emits silence when gated. Source reads,
DSP, Go callbacks and control messages stay outside the real-time callback.
Only the device timestamp mapping produces transport acknowledgements.

Control cancellation awaits both its original reply and an abort/close fence,
including a successful open racing cancellation. Device close fences pending
source reads before Go releases the callback and PCM cursor; the session closes
the borrowed reader afterward. Mid-attachment cancellation remains `cancelled`,
consumes no successful sequence, and advertises no live capability.

Recovery rearms a fresh context from the user's action, waits for old-device
detach before installing the new host, verifies/attaches the source again, and
stops at the saved selection start. The UI displays the actual returned command
sequence rather than assuming one sequence per button press. Close playback
returns to selection preview. Reopening a source accepts its new sequence-zero
snapshot; only replies within one session are filtered by sequence. An old
session's higher sequence cannot block a newly verified source. Workspace close and worker failure close the
context once. If context close rejects, failure publication waits for a
suspension fence; rejection of both fences never publishes false quiescence.

## Evidence

`node scripts/check-browser-live-binding.mjs` uses the real Go/WASM session,
browser live factory, verified source reader, C++/WASM processing worker and
production worklet/host modules. Its platform shims supply synthetic device
positions, so this is integration evidence, not physical-device or listening
evidence. It checks:

- No queued/callback-only frames count as completed loops.
- Complete presented traversals, live speed/pitch edits through fresh generations,
  pause position held across idle polls,
  seek followed by an uncounted partial traversal, then a complete loop.
- Stop resets runtime counts/position; detach closes output once.
- Invalid device timestamps close output before Go reports an error, followed
  by successful playback with a fresh attachment.
- Cancellation while host open is held waits for close, consumes no sequence,
  and permits a subsequent attachment.
- Original reads stay within 64 KiB and export preserves saved settings without
  runtime transport fields.

CI runs this against its freshly built Go and C++ WASM binaries for 0.75/+3,
1/0, 0.25/−12 and 2/+12 and retains a report for each profile. `check-practice-facade.mjs` additionally checks user gesture,
setup cancellation, old/new host recovery ordering and exactly-once close using
the actual facade and host controller. Portable Go tests cover acknowledgement
bounds and attachment cancellation with device-before-reader ownership.

A local browser check used the generated three-second 48 kHz demo with a
0.25–0.75 second loop, speed 0.75 and pitch +3. Playback reported 32 complete
loops, paused at 0.488 source seconds, then sought to 0.375 and resumed. A second
pause reported 39 complete loops at 0.415 seconds. These are browser API position
estimates, not independent speaker measurements or music-quality evidence.
A deliberately five-source-frame loop exhausted lookahead, showed an explicit
error, and recovered through the workspace button to stopped position/counts.
The normal-speed follow-up exposed repeated callback render positions: frame
441216 was supplied again after the preceding block already ended at 441344.
The callback now caches exactly one render block (up to 8192 frames/channel)
and reuses its PCM for an identical frame/size request. It neither consumes new
queue data nor publishes another timeline/counter entry. Pause requires a distinct
render boundary, and repeated EOF tails preserve their original samples. Other
overlaps, size changes and corrupt/exhausted timelines still fail closed.

Callback failure details use reserved fixed queue-header words and setup-time
scratch space, with no callback logging/messages/errors allocated. The main host
reports the detail only after its context-close fence; local console diagnostics
do not change transport policy or UI counts. The production-worklet threaded test
now repeats blocks while retaining exact PCM/counters, and focused checks cover
stereo repeats after entry retirement, pause, EOF and rejected different overlaps.

After this fix an actual 1/0 browser run reached 690 complete half-second loops,
then paused through a remapped key at 0.506 source seconds. This demonstrates the
host workflow with browser position estimates; independent physical presentation
accuracy and listening quality remain unverified.


## Remaining gates

This slice supports original-rate mono/stereo integer WAV in an isolated page
with AudioWorklet and output timestamps. Other browser codecs, durable storage,
offline packaging, mobile/native live devices, keyboard workflows, physical
latency/listening and long-session browser evidence remain roadmap work. A
schedule can exhaust its two-entry lookahead for very short loops or delayed
control polling; the worker fails closed instead of silently repeating PCM or
counting silence. Short-loop continuity requires a separate scheduling slice.
