# Persistent native scheduling control (partial #20)

The native adapter can now retain one isolated helper and its two borrowed-source
decoder/DSP slots across serialized metadata, reset and schedule requests.
Opening verifies the helper's `scheduledControl: 1` capability and matches native
source dimensions with the caller's inspected metadata. The opening context
bounds only opening/handshake; later cancellation of that context does not own
or terminate a successfully opened worker. Calls use their own contexts.

Protocol v1 requests are exactly 128 bytes and responses exactly 640 bytes,
little-endian, with fixed magic/version and monotonically increasing request IDs.
Generation/schedule identities retain all 64 bits. Schedules carry source/output
bounds, fade, speed and semitones from the shared Go engine. Responses contain
bounded native metadata or a 512-byte terminated error field. Reserved fields,
malformed replies, dimension changes, unsupported versions and truncated input
fail explicitly. Native argument rejection leaves slots unchanged and preserves
framing. The helper supports explicit close or clean stdin EOF; the Go owner
unconditionally cancels/kills/waits on Close so a stalled request cannot block
teardown. Only owned pipe/process resources are closed, never the caller reader.

No PCM crosses this IPC. This scheduling worker is deliberately not an
`engine.OutputDevice` or `practice.LiveDeviceFactory`: metadata/pending/generated
state cannot acknowledge audio presentation. Real device start/suspend, native
producer/queue integration, backend presentation clocks, resampling and shell
packaging remain #20 work. No native live capability is advertised by this slice.

## Evidence

- Actual helper tests retain one PID through 100 generations for each generated
  WAV/licensed FLAC/MP3 source, including original-path replacement after opening.
  Two slots accept schedules; a third and stale resets reject without changing
  state. Generation/sequence values above 2^53 and generation UINT64_MAX round-trip
  exactly. Original reader position/ownership survive worker close.
- Controlled child processes exercise malformed IDs/dimensions/reserved fields,
  invalid status/error text and truncated responses. Opening and in-flight calls
  are cancelled while actually stalled; concurrent Close interrupts an active
  request and waits for child exit. Failed workers cannot resume operations.
- Native protocol close exits cleanly; a truncated input envelope fails rather
  than interpreting partial data. Decoder teardown precedes caller release.
- Mandatory native-backed Go CI runs on Windows/macOS. Linux's required sanitizer
  job builds the actual helper and runs persistent native control tests with
  ASan+UBSan/leak detection. Local Release/native/Go/race and macOS-supported
  ASan+UBSan checks cover the slice.
