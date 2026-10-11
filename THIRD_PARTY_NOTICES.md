# Third-party components

Riffine's own code is MIT licensed. Dependencies keep their own terms.
No third-party source or commercial recordings are committed in this repository.

| Component | Pinned revision | License | Purpose |
| --- | --- | --- | --- |
| [Signalsmith Stretch](https://github.com/Signalsmith-Audio/signalsmith-stretch) | `a670068d9aeb64913331d5cc29337b19a457a7df` | MIT | Independent pitch/time processing |
| [Signalsmith Linear](https://github.com/Signalsmith-Audio/linear) | `de55e6a50ffcf6f8f43f649692d94691c7025151` (0.6.4) | MIT | DSP/FFT dependency |
| [Emscripten](https://github.com/emscripten-core/emscripten) | `4.0.14` (SDK `eff90ca04a3785f571a8095b3a42b63799cf384a`) | MIT alternative selected; musl MIT and LLVM runtime license with exceptions | Browser C++ processing module/runtime |
| [miniaudio](https://github.com/mackron/miniaudio) | `f40cf03f80cdb7e741d43e53b7e706e8c1394bcf` (0.11.23) | MIT-0 alternative selected | WAV/FLAC/MP3 codecs and native device output |

CMake downloads these revisions and installs their complete license texts into
`licenses/`. Distribute that directory with native binaries, together with this
file and Riffine's LICENSE. Build scripts and CI also copy the Go toolchain's
LICENSE and PATENTS. The CI artifact includes these notices. Release packaging
must add a complete dependency inventory.

The browser DSP build copies complete Signalsmith, Emscripten, musl, libc++, libc++abi, libunwind and compiler-rt
notices into `web/public/practice/licenses/`, together with Riffine's license and
this inventory. Keep those files with the generated browser module and WASM.
The Emscripten notice includes its bundled runtime's additional components.

Future Wails, FFT acceleration, codecs or DSP alternatives require their own
review. Do not silently add GPL/commercial DSP dependencies to an MIT-only build.
No rights to users' imported recordings are granted by this repository's license.

Riffine is a working name. Repository availability is not trademark clearance.
Perform appropriate trademark/name checks in target markets before branding a
release. This project is independent of Transcribe! and its publisher.
