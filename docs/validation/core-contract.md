# Portable core foundation validation

Date: 2026-10-10. Scope: roadmap #12, API v1 and unchanged project schema v1.

## Automated evidence

`go test ./...` executes the shared `session-v1.json` trace against an in-memory
host and the filesystem adapter. Both compare commands, rejected commands,
snapshots, events and renderer source-frame requests. Additional tests cover
strict/bounded project JSON, verified relocation, parameter units, malformed WAV,
integer depths, stereo peaks, cancellation, resource closure, failed publication,
exclusive concurrent creation, event overflow/resynchronization and command races.

Core production imports were inspected using `go list`: no filesystem APIs,
platform/UI packages, networking clients or native adapters are imported by core.
Core WAV and project tests operate on memory readers/writers.

Passed on macOS arm64 with Go 1.27.2 and AppleClang 21:

- `go test ./...`, `go vet ./...`, `go test -race ./...`, `gofmt` and
  `git diff --check`.
- CLI build and `GOOS=js GOARCH=wasm go test -c ./core/practice` (compilation only).
- CMake configure/build/install using the repository's immutable dependency pins.
- `sh scripts/test.sh --native` with the built helper: the native DSP integration
  cases ran, including independent speed/pitch, stereo/Unicode paths, short loops,
  collisions and cancellation. They were not skipped in this run.

A generated-tone CLI workflow passed: `tone` → `doctor` → `import` → `set` → `loop`
→ `inspect --verify` → `waveform` → `render` → collision rejection → verified
`relink`/reopen. The 48 kHz source used a [0.25 s, 0.75 s) selection at 0.5x speed
and -2 semitones; output contained exactly 48,000 frames. Original file SHA-256
remained unchanged after processing and relocation. All media was generated and
all temporary workflow files were cleaned up.

The native helper was built and exercised on macOS; Windows CI remains the check
for Windows publication/native behavior. Local Go differs from CI's Go 1.24, so
this record does not claim the CI matrix ran here.

## Support limits

This change adds no DSP algorithm, live audio callback, output device, codec,
browser binding, mobile binding, shell, routine or project-schema migration.
Clock/streaming/output ports are provisional. Device listening/latency/underruns
remain unvalidated in this task. A pure-Go test run skips native integration
unless `RIFFINE_AUDIO_HELPER` names a built helper; distinguish that from an actual
native integration run. Browser/mobile execution is the next gate in #13.

Filesystem exclusive publication currently requires hard-link support. The staged
file is synced; parent-directory durability and in-place recovery are not claimed.
Future storage/revision work in #16 must cover those requirements.
