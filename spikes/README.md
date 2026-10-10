# Cross-host feasibility harnesses

These are development experiments for roadmap #13, not production clients.
One Go harness executes the shared `core/practice/testdata/session-v1.json` fixture
through native, worker/WASM and C ABI entry points. JavaScript/C callers only
supply input and compare reports; all validation and acknowledgement rules stay
in Go. Renderer and output ports are fakes and no browser audio is produced.

Start with `sh scripts/build-host-spike.sh portable`, then
`node spikes/browser/check.mjs`. Serve `build/host-spike` to open the browser
worker harness. `build-host-spike.sh native`, `ios` and `android` build C ABI
callers with explicit SDK/toolchain requirements. Outputs stay ignored in `build/`.

See [ADR 0003](../docs/adr/0003-cross-host-feasibility.md) and
[the evidence and reproduction record](../docs/validation/cross-host-spike.md).
Issue #13 remains open for mobile runtime and C++ WASM validation.
