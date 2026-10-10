# Contributing

Use a feature branch and a pull request. Describe user-visible behavior and
validation. Keep clients out of this initial core/CLI phase.

Run `go test ./...`, `go vet ./...` and `gofmt` before submitting changes. Changes
to native audio require `scripts/test.ps1 -Native` or `sh scripts/test.sh --native`
after building. Native integration tests must run in CI, not silently skip.
Use generated or explicitly licensed audio fixtures; do not commit personal songs,
projects, credentials, build output or toolchain downloads.

Keep dependencies pointed inward: CLI -> core and adapters; adapters -> core
ports; core must never import CLI, Wails, networking clients or native adapters.
Public Go APIs are experimental until v1. Project format changes need an explicit
migration and fixtures. Never reinterpret saved tempo or loop time units.

Changes to DSP dependencies must update immutable CMake revisions and notices,
run native tests on Windows/macOS, and record any listening/latency changes.
Do not put allocation, locks, logging, decoding or Go callbacks in a future
real-time device callback. Use bounded buffers and a worker instead.
