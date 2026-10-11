# Bounded PCM and library indexing foundation

Date: 2026-10-11. Partial progress on #17; prerequisite work for #18.

`audio.WAVDecoder.OpenPCM` implements the shared streaming-decoder port. It
borrows the reader and owns its cursor and fixed temporary buffer. Reads accept
1–8192 complete interleaved float32 frames, preserve original source coordinates
and support exact seeks including EOF. Cancelled and rejected calls do not advance
the cursor. Closing the stream is idempotent and leaves its borrowed reader open.
The owner closes the resource after the stream. Reads stay on a worker.

Inspection still fingerprints the entire original file once on opening; the
cursor does not decode or hold the entire song. Integer PCM 16/24/32-bit, mono and
stereo remain the only supported formats. External mutation during a stream is
outside the stable-resource contract.

`core/library.Index` consumes host enumeration incrementally, emits at most 256
results per batch and retains at most 100,000 canonical identities. It retains
metadata dimensions for duplicate consistency, not audio or alias lists. A
candidate currently being inspected remains pending after cancellation; completed
rows remain committed, and the next call retries only the pending candidate.
Every opened reader is closed, including failure/cancellation paths. The host
owns and closes enumeration, supplies interruptible resource reads and serializes
index calls. Capacity or inconsistent identity metadata fails explicitly.

Source identity is the SHA-256 of complete original bytes, including headers and
tags. Encodings of the same musical content are separate identities. Identical
bytes through another locator produce a duplicate result. Tags affect display,
not identity: whitespace is trimmed, an absent title falls back to filename
without its extension, and missing artist/album remain absent. Tag parsers belong
to hosts; the CLI currently supplies no tags. Every display/handle field is bounded
at 4096 UTF-8 bytes. Invalid or oversized values are omitted from error results.
Malformed, missing and unsupported files produce `unavailable`, rather than
inventing metadata from an extension. Per-locator display tags are not persisted
as canonical metadata by this index.

## Try the CLI

Create a UTF-8 JSON array of source locators in `sources.json`; relative locators
are resolved against that list's folder:

```json
["tone.wav", "copy.wav", "unsupported.mp3"]
```

```sh
sh scripts/build.sh
bin/riffine scan --sources sources.json
```

The list is bounded to 1 MiB and 100,000 locators. Output is newline-delimited
versioned JSON batches containing actual decoder formats, results and a `done`
flag. The CLI retains only the bounded input list and current batch. Large folder
clients can instead supply lazy host enumeration through the core Source port.
No source or project is written by scanning.

## Validation and remaining acceptance

Golden sample checks cover stereo endpoints, silence, sign extension, 16/24/32-bit
samples, exact seek values, EOF, read bounds, cancellation and borrowed-reader
ownership. Library checks cover Unicode/missing-tag fallback, aliases with false
extensions, malformed/missing resources, cancellation retries and capacity failure.
The shared Go/native/WASM/mobile fixture now exercises PCM seeking and library
identity/deduplication. Local Go tests/vet/race and twenty native/Go-WASM report
comparisons passed.

#17 remains open: generic codec metadata/project migration, FLAC/MP3 adapters,
compressed seek offsets, host tag parsers and client capability integration still
need implementation. This foundation does not advertise compressed decoding.
