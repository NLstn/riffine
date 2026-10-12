# Borrowed native preview (partial #17/#20)

The CLI preview now verifies and previews one retained filesystem resource through
`engine.SourcePreviewer`. Replacing the original path between inspection and
playback no longer causes the native helper to reopen another file. The helper
inherits a read-only resource, retains its own decoder cursor and keeps decoded
PCM, DSP and device callbacks native. Only a bounded diagnostic report crosses
the process boundary. Caller readers remain owned by the Go session.

The native C ABI adds `riffine_preview_open_handle` and `riffine_preview_handle`;
the helper adds `preview-handle` and advertises `borrowedPreview`. Older helpers
with borrowed decoding remain valid decoders, but the adapter/CLI rejects their
unavailable preview capability with a rebuild message. The explicit standalone
path preview API remains available. Cancellation waits for helper exit before
the caller may release its original reader. Invalid ranges are rejected before
launching the preview worker; returned dimensions must match inspected metadata.

## Validation

Mandatory Go/native integration covers generated/licensed WAV, FLAC and MP3 at
25%, 100% and 200% speed with pitch changes. It opens and inspects a Unicode source,
renames it, writes invalid audio at the old path and successfully previews the
retained original. It verifies parent file position and fingerprint, rejects a
range beyond the source, and tests cancellation before launch and during preview.
The original reader remains usable after cancellation and decoder/device teardown.

The native scheduled test compares borrowed preview PCM sample-for-sample with
the existing path kernel for all three formats and verifies independent cursor
ownership. The C-compiled ABI rejects invalid handles with cleared results.
Release native/Go checks and ASan+UBSan cover these paths. CI runs the actual native
integrations on Windows and macOS as well as the Linux sanitizer checks.

This is bounded one-selection preview. Its report describes callback submission,
not hardware presentation. Persistent native live transport, measured backend
presentation clocks and desktop packaging remain #20 work. Inputs must stay
immutable while processing; retaining a handle protects file identity across path
replacement, not modification of that same object in place.
