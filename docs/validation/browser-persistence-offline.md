# Device projects and offline browser practice (#19/#26)

The production browser app now caches a complete version of the UI, Go/WASM
control worker, shared C++/WASM processing, decoder workers/modules, generated
demos and runtime notices. User audio and project requests are not cached by the
service worker. The LibAV corresponding-source archives remain in the production
package; they are not required to play and are not precached.

`npm run build` requires the generated core and DSP assets. Missing engines fail
that build rather than publishing an apparently complete offline application.
`npm run dev` remains usable for library/UI work without a processing build.
Serve the production root over localhost or HTTPS with COOP `same-origin` and
COEP `require-corp`; Vite preview supplies these headers. A production host must
return the service worker as JavaScript and assets with their proper MIME types.

Wait for **App ready for offline practice** before disconnecting. A failed
installation discards only its incomplete version and keeps the previous complete
cache. Updates wait until all old Riffine tabs close; they never replace the
engine in a running session. Reopen at the same origin/port. Browser storage can
be evicted or cleared, so downloaded projects remain the portable backup.

## Save, reopen and recover

**Save on this device** calls the shared core's exclusive `SaveAs` for an imported
or downloaded project. It saves settings, phrases, markers, notes and plans, not
audio or runtime counters. Reopen that copy from **Projects saved on this device**
with the matching original audio to begin editing it in place. If another song
is already selected, **Choose original and reopen** selects the intended source
without replacing the current session until identity verification succeeds. Downloads and
previous-version recovery documents remain read-only; saving one creates a new
copy. This follows the existing core save-as/reopen contract without replacing a
live transport during a save.

An opened device project uses `Session.Save` with its exact loaded-byte SHA-256
revision. The IndexedDB host calculates new revisions outside the transaction,
checks the current bytes/revision and commits the new document and separate
previous-version copy in one read-write transaction. Concurrent tabs cannot both
commit the same revision. Conflicts retain the active edits: download them or
**Save a new device copy**, then explicitly reopen the saved version. There is no
automatic refresh/retry/merge that can replace someone else's saved work.

Recover through **Reopen previous version**, verify the original audio again,
then save a new device copy. Recovery copies are retained until explicitly
removed. Removing a primary copy also removes its associated recovery copies;
removing a recovery copy leaves the primary untouched. The open copy cannot be
removed from this workspace. Removing a project never changes the original audio.

IndexedDB unavailability, blocked upgrades, quota failures, cancellation and
missing copies have actionable errors. Failed transactions publish neither a
partial project nor a partial recovery copy. Save cancellation races can still
commit before cancellation arrives; refresh/reopen rather than assuming the save
did not happen. Selected audio permission/read failures and identity mismatches
come from the resource adapter/shared core. Source files are reselected after a
reload, providing an explicit relinking fallback on browsers without persistent
file-handle permissions. No account or network audio processing is involved.

## Evidence

Run after the core/DSP/browser build:

```sh
node scripts/check-browser-storage.mjs
node scripts/check-browser-saved-projects.mjs
node scripts/check-browser-offline.mjs
node scripts/check-practice-workspace.mjs
```

IndexedDB tests cover fresh-store reads, exclusive publication, competing writers,
exact prior bytes, failed/cancelled transaction rollback, quota failures, document
bounds and deletion of recovery copies. The real Go/WASM adapter check covers
save-as/reopen, revision updates, identity mismatch retaining the active session,
conflicts retaining edits and unchanged command sequencing. Service-worker checks
cover all generated assets, incomplete installs, cache cleanup, offline responses,
isolation headers and exclusion of user resources. CI builds actual processing
assets before these checks; it does not use placeholder WASM files.

On 2026-10-13, the local Codex Chromium browser loaded the production app,
imported the generated MP3, saved 60% speed/+3 pitch/notes, reopened it and saved
65% with a previous-version copy. After stopping the localhost server, it reloaded
the app, listed the saved copies, reselected the original MP3, verified identity,
restored the settings/notes and completed a selection preview. This checks one
Chromium runtime and a short generated fixture, not musical listening, hardware
latency, 30-minute sessions or Chrome/Edge/Firefox/Safari release acceptance.
