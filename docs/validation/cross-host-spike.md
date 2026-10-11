# Cross-host feasibility slice validation

Date: 2026-10-10. Scope: an initial executable slice of #13, following #34.
Host: macOS arm64, Go 1.27.2, Node 25.2.1, AppleClang 21, iPhoneSimulator26.5 SDK.

## Results

| Path | Evidence | Status |
| --- | --- | --- |
| Native Go | Shared fixture and deterministic complete JSON report | Passed |
| Go/WASM in Node worker | 20 reports compared exactly with native Go | Passed |
| Go/WASM in actual browser worker | All fixture checks, identity, project round-trip, events, cancellation and fake acknowledgement | Passed |
| macOS in-process Go C ABI | C caller's full JSON report compared exactly with native Go | Passed |
| iOS arm64 simulator Go C ABI | Go archive compiled; C caller linked to Mach-O executable using simulator SDK | Build/link passed; runtime unverified |
| Android arm64 Go | ELF PIE executable compiled for Android's linker64 | Compilation passed; runtime unverified |
| Android Go C ABI | Build recipe rejects missing `ANDROID_NDK_HOME`; no NDK installed or adb device connected | Outstanding |
| Native C++ DSP | Direct C++ caller linked existing pinned library and rendered a generated selection | Passed offline |
| C++ DSP/WASM | No Emscripten toolchain/C++ WASM sysroot available | Outstanding |

The source identity in native/C/browser reports was
`9c7dd049d22565e824a57008492fc51fae56cf5afff6c963b28e6ed491441a6e`.
The version-1 project survives save/reopen without changing settings or identity.
State events have sequences 1, 2, 3. A cancelled command changes no state.
A fake scheduled loop covers source frames [12000, 36000) at 0.75x and acknowledges
output end 32000; partial/stale/duplicate acknowledgement cannot emit a second
completion. `traversal_completed` is a control experiment, not audible playback.

## Measurements

One browser observation after the final bounded build: startup 23.8 ms, full
fixture execution 16.7 ms, initial WASM linear memory 4,063,232 bytes and final
5,242,880 bytes. These local warm-cache measurements are samples, not budgets.

The final Go/WASM bundle is 5,126,650 bytes raw and 1,401,624 bytes gzip (level 9).
One 20-run Node worker comparison measured startup 43.1 ms, first fixture 23.9 ms,
later fixture runs roughly 1.3–9.4 ms and round trips roughly 1.4–9.5 ms; final
linear memory was 9,175,040 bytes. This measures a whole synthetic fixture,
including tone generation/hashing, JSON and worker scheduling. It is not a
single command's latency or total process RSS, and does not prove long-session
memory bounds. `check.mjs` emits raw measurements for repeated investigations.

Twenty native C ABI process launches plus fixture execution measured median
2.9 ms (min 2.7, max 12.4). This is cold process overhead, not a mobile call budget.
The fixture API bounds input to 1 MiB, steps to 256 and synthetic tone length to
192,000 source frames. It refuses oversized inputs rather than claiming support
for an arbitrary recording. Those bounds are spike limits, not product limits.

The in-process native DSP probe rendered [12000, 36000) at 0.5x, -2 semitones to
exactly 48,000 output frames; source SHA-256 remained unchanged. The probe needed
`-framework Accelerate` for the existing macOS FFT build. No musical listening,
mobile audio device, AudioWorklet or streaming quality result is claimed.

## Reproduction

From the repository root (use writable Go caches if required by the host):

```sh
go test ./...
go vet ./...
go test -race ./...
sh scripts/build-host-spike.sh portable
node spikes/browser/check.mjs
RIFFINE_SPIKE_OUT=build/host-spike/native sh scripts/build-host-spike.sh native
build/host-spike/host-spike < core/practice/testdata/session-v1.json > build/host-spike/go-report.json
build/host-spike/native/mobile-runner core/practice/testdata/session-v1.json > build/host-spike/c-report.json
diff -u build/host-spike/go-report.json build/host-spike/c-report.json
python3 -m http.server 4174 --bind 127.0.0.1 --directory build/host-spike
```

Open `http://127.0.0.1:4174/` in a browser: the page displays the report and metrics.
Worker resource input and project storage stay in memory. Audio is generated in
Go; no song, project or account is uploaded. The generated web assets include
the matching toolchain's `wasm_exec.js` so runtime/toolchain versions cannot drift.

macOS with Xcode simulator SDK:

```sh
RIFFINE_SPIKE_OUT=build/host-spike/ios sh scripts/build-host-spike.sh ios
# A functioning booted simulator is required for runtime evidence:
xcrun simctl spawn booted "$PWD/build/host-spike/ios/mobile-runner" "$PWD/core/practice/testdata/session-v1.json"
```

No functioning simulator was available in this run: inventory was bounded to
15 seconds and timed out; an earlier stalled inventory process was terminated.
Do not treat the linked binary as proof that the spawn recipe passed.

Android:

```sh
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -o build/host-spike/android-go ./cmd/host-spike
# Point these at an existing NDK; the script does not download/install SDKs.
ANDROID_NDK_HOME=/path/to/ndk RIFFINE_NDK_HOST_TAG=darwin-x86_64 RIFFINE_SPIKE_OUT=build/host-spike/android sh scripts/build-host-spike.sh android
adb push build/host-spike/android/libhost-spike.so build/host-spike/android/mobile-runner core/practice/testdata/session-v1.json /data/local/tmp/
adb shell 'cd /data/local/tmp && LD_LIBRARY_PATH=. ./mobile-runner session-v1.json'
```

Use `linux-x86_64` for that NDK host. Android requires an arm64 API 24 compiler
for this recipe. Runtime execution and full report comparison remain required.

Native DSP, after the normal CMake native build:

```sh
c++ -I native/include spikes/mobile/dsp_probe.cpp build/native/libriffine-dsp.a -framework Accelerate -o build/host-spike/dsp-probe
# Generate source.wav with the CLI tone command; output.wav must not exist.
build/host-spike/dsp-probe source.wav output.wav
```

The link recipe shown is macOS-specific. Use CMake's platform libraries on Linux.
Shared C++ WASM build/run and its recipe remain outstanding; Go/WASM is not a
substitute for that gate.

## Automated coverage and remaining acceptance

Go tests/vet/race and fixture checks passed locally. The separate
`Host feasibility spike` workflow checks native Go ↔ Node/WASM ↔ native C ABI
report equality on Linux and iOS simulator build/link on macOS using Go 1.24.
It does not claim browser UI or mobile runtime tests, and it is not a substitute
for the existing aggregate CI gate.

#13 stays open: complete Android C ABI build/run, iOS runtime round-trip, C++ DSP
WASM execution and device/lifecycle/processing measurements before selecting
production bindings. Existing #12 PR CI completed successfully during this work.

## Runtime follow-up, 2026-10-11

The earlier simulator inventory timeout was resolved using the appropriate host
execution permissions. Actual iOS simulator C-to-Go runtime conformance now passes
20 repeated calls, including the newer session/transport/ladder traces. See
[mobile runtime evidence](mobile-runtime.md) for measurements, scripts, Android CI
coverage and the remaining production/audio gates. The initial table above remains
a record of the 2026-10-10 run rather than current coverage.

## C++ WASM follow-up (2026-10-11)

The earlier missing-toolchain observation is superseded by the executed native
and WASM streaming C ABI probe in [wasm-streaming-dsp.md](wasm-streaming-dsp.md).
It includes actual browser worker execution and twenty native/WASM comparisons;
AudioWorklet playback and device timing remain separate acceptance gates.
