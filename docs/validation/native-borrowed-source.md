# Borrowed native source identity (partial #17/#20)

Native compressed metadata/analysis formerly resolved the resource's file path
again. Replacing that path after opening the Go reader could mix the old reader's
fingerprint with another file's metadata/PCM. The adapter now lends the original
filesystem-owned read-only file to the helper. No path lookup occurs in the new
`decoder-info-handle` / `decoder-read-handle` operations.

POSIX uses an inherited descriptor; Windows creates a scoped inheritable duplicate
and supplies an explicit process handle list. The helper retains its own read-only
duplicate/reopen and performs offset-based encoded reads bounded to 64 KiB. POSIX
uses pread. Windows uses a private overlapped reopen/event, waits for each read to
finish and never reuses its buffer/event while I/O is pending. Encoded source
bytes and decoded PCM remain on non-realtime workers. Live processing can use the
same resource directly through `riffine_scheduled_open_handle` without Go samples.

The native resource has an independent logical byte cursor for each decoder.
Header precision parsing also reads that same resource. Seeks/decoder teardown
never change the parent's file position or close its handle. Runtime callback
errors return through the native C ABI rather than throwing across C callbacks.
All read/cancel/final-Wait ownership fences in the Go analysis adapter remain.
The format report adds `borrowedSources: true`; the adapter rejects helpers without
that capability and requests a rebuild instead of falling back to path lookup.

Filesystem reader opening now shares deletion on Windows, using the same tested
read-only/long-path helper as peak-cache readers. Thus relocation/replacement can
leave an open reader attached to its original object. This does not update saved
resource references; reopening a moved original still requires explicit relink.
The input resource must remain immutable while processing; handle identity does
not protect a POSIX file from another process modifying its contents in place.
Legacy direct-path preview/render operations remain a separate ownership boundary
and are not changed or claimed to borrow their source by this slice.

## Evidence

Actual Go/native FLAC and MP3 regression tests open a Unicode original, inspect and
seek it, rename it and create invalid replacement bytes at the old path, then
repeat metadata/fingerprint and PCM operations through the retained resource.
Results equal the original; the parent's file position remains 123 and its reader
remains open after helper teardown. A platform filesystem test covers retained
reader identity across replacement. Controlled stalled-worker tests still verify
read/final-Wait cancellation, and a capability fixture rejects an old helper.

The native scheduled-worker test now runs its complete WAV/FLAC/MP3 profile/reset
suite through both paths and borrowed handles, checks both decoder cursors and
verifies the caller's handle/position after worker close. The C-compiled ABI test
rejects invalid handles with cleared result ownership. Local Release and ASan+
UBSan checks passed; Windows runtime verification comes from CI. Windows Go code
cross-compiled locally. The full eight native CTests, native-backed Go integrations,
Go tests/vet/gofmt and native/filesystem race checks passed locally. No source audio
was overwritten; all audio used here is generated/explicitly licensed fixtures.

The platform read behavior follows Microsoft's
[ReadFile documentation](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-readfile)
and [ReOpenFile documentation](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-reopenfile).
A pending overlapped operation retains its buffer/state until completion; the
resource wrapper follows that lifetime rule. Persistent device IPC, real backend
presentation clocks, desktop shell and physical listening remain roadmap work.
