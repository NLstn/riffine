# Portable sessions: project v2 (issue #16, first slice)

Project v2 adds source-time regions, section/measure/beat markers, notes, named
loops referencing regions and bookmarks. IDs are stable opaque ASCII keys (1–128
letters, digits, hyphen or underscore), unique across collections. Positions and
ranges remain source seconds; measure/beat ordinals are explicit one-based labels,
not an inferred tempo map. Notes accept UTF-8 and have a 16 KiB per-note limit;
labels have a 1 KiB limit, and collections total at most 4096 entries. The document
and command decoder remain bounded at 1 MiB. Edits that cannot serialize within
that bound are rejected without consuming their sequence.

The strict v1 migration preserves name, locator, full source fingerprint/metadata,
speed, pitch and exact loop seconds. A legacy range containing at least one frame
becomes `region-v1-loop` with the named loop `loop-v1`, deterministically, whether
or not looping was enabled. A valid disabled sub-frame range remains in settings
without becoming a region. Migration is in memory; reading never rewrites the
original. Save-as emits the current project version into a new file, and older applications must reject newer formats. Project v3 later adds practice-plan definitions.
Unknown future versions and fields fail explicitly.

`set_annotations` replaces the annotation collection atomically under the session
control lock. Missing region references, duplicate IDs, invalid marker kinds or
ordinals and ranges outside the track are rejected. The command accepts annotations
only. Caller objects and returned snapshots are copied so clients cannot edit
session state by retaining slice references. Relinking verified original bytes
preserves annotations and their IDs. Annotations do not imply a tempo analysis,
change the active loop or create playback events.

## Try it

Create a JSON file, for example `notes.json`:

```json
{
  "notes": "Practice the pickup slowly",
  "regions": [{"id":"intro","label":"Intro","start":0.1,"end":0.7}],
  "markers": [{"id":"bar1","kind":"measure","label":"Bar 1","position":0.1,"measure":1}],
  "loops": [{"id":"repeat","label":"Intro","regionId":"intro"}],
  "bookmarks": [{"id":"tomorrow","label":"Resume","position":0.3}]
}
```

For a track at least one second long:

```sh
bin/riffine annotate --project song.json --out annotated.json --annotations notes.json
bin/riffine inspect --project annotated.json --verify
```

No existing document or source audio is replaced. Both original and v2 projects
remain separately recoverable. Select a named loop by applying its region bounds
through the existing core `set_loop` command; no second loop engine is introduced.

## Evidence and remaining work

`core/project/testdata/v1.json` and `v2.json` define the migration pair (synthetic
metadata fingerprint). Tests cover golden migration, disabled sub-frame bounds,
Unicode notes, full round-trip, invalid references/ordinals/IDs, size bounds,
caller/snapshot ownership, relink, cancelled/failed edits, failed save-as and
original byte preservation. A CLI filesystem flow imports a generated tone,
applies annotations, reopens and verifies identity, and rejects overwriting.
The shared host harness round-trips annotations through the same Go service in
native Go, WASM and the C archive; host glue implements no annotation rules.

Remaining #16 work: atomic updates to an existing document with revision/conflict
protection and recovery; a browser production storage adapter; persistence of
practice-plan definitions when their schema is implemented. Those acceptance items
remain open. Save-as is already exclusive and atomic but is not an in-place update.
