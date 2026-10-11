# Session saving, revisions and recovery (issue #16)

`Session.Save` updates a document opened through the shared service. Its revision
is the SHA-256 of the exact loaded JSON bytes, separate from both audio identity
and edit command sequence. Every update compares that revision; stale sessions
receive `revision_conflict` and keep their unsaved edits. Hosts implement the
optional `resource.UpdatingStore` port. Imported sessions and hosts without this
port must use exclusive save-as and reopen. Saves do not count loops or change
edit command sequence. The snapshot exposes the last successfully saved revision.

The filesystem host rejects non-project files and final-component symlinks. It
resolves parent directory aliases and acquires an exclusive `.riffine-lock`
directory beside the project. Competing Riffine writers receive `document_busy`;
a retry after the winner commits receives a revision conflict. External editors,
source replacement, hard-link aliases and uncooperative writers are outside this
contract. There is no automatic overwrite or merge of someone else's revision.

Before publication, the host creates and syncs a separate, exclusive previous
revision copy named `<project>.recovery-<revision>.json`. An existing recovery file
must exactly match the previous bytes; unrelated files are never replaced. It
stages and syncs the new document on the same volume, checks cancellation, then
replaces the target with POSIX rename or Windows
[ReplaceFileW](https://learn.microsoft.com/windows/win32/api/winbase/nf-winbase-replacefilew).
There is no truncate/delete-before-rename fallback. Filesystem limitations fail
explicitly. Unix directory metadata is synced before commit. Windows has no
portable directory fsync; abrupt power-loss durability still needs target-storage
validation. Process interruption before commit leaves the original readable;
even an ambiguous OS replacement failure leaves the separate valid recovery copy.

## Try it and recover

```sh
bin/riffine save --project song.json --speed 0.6 --pitch 3
```

The output names the committed revision and previous valid recovery document.
`save` without settings also explicitly migrates a loaded v1 document to v2. Other
CLI outputs remain exclusive. All stored notes, markers and regions are preserved.

Open the reported recovery document using `inspect --project <recovery>` or the
shared service. Use `SaveAs` to restore it into a **new** project. Its relative
source locator still resolves because recovery copies remain in the same directory.
Recovery never automatically rolls the active project back or replaces source audio.

Normal return removes the save lock and temporary file. A killed/crashed process
can leave the lock directory behind: existing documents and recovery copies remain
readable. Confirm no writer is running before manually removing that empty lock
and reopening. There is no timeout-based lock stealing. A failed save must not be
silently retried with a fresh revision; display the conflict and let the user
reopen or save their unsaved edits under a new name. Retained recovery copies are
not automatically deleted; a storage-retention policy belongs to subsequent host UX.

## Evidence

Filesystem tests check Unicode paths, successful replacement with identical prior
bytes in recovery, stale revisions, gated competing writers, interrupted and
cancelled commits, safe retry, stale crash locks, conflicting recovery files and
refusal to overwrite generated source audio. Core memory-host tests check revision
propagation, recovery, unsupported hosts and unchanged state on failed saves. A CLI
workflow edits saved settings, verifies source bytes, reopens the prior revision
and exports it into a new document. Windows compilation is checked locally and
Windows/macOS/Linux behavior runs in CI.

Production browser persistence and later practice-plan schema fields remain
separate work. No power-loss or real-device storage claim is established by these
tests.
