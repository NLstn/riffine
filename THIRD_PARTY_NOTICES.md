# Third-party components

Riffine's own code is MIT licensed. Dependencies keep their own terms.
No third-party source or commercial recordings are committed in this repository.

| Component | Pinned revision | License | Purpose |
| --- | --- | --- | --- |
| [Signalsmith Stretch](https://github.com/Signalsmith-Audio/signalsmith-stretch) | `a670068d9aeb64913331d5cc29337b19a457a7df` | MIT | Independent pitch/time processing |
| [Signalsmith Linear](https://github.com/Signalsmith-Audio/linear) | `de55e6a50ffcf6f8f43f649692d94691c7025151` (0.6.4) | MIT | DSP/FFT dependency |
| [Emscripten](https://github.com/emscripten-core/emscripten) | `4.0.14` (SDK `eff90ca04a3785f571a8095b3a42b63799cf384a`) | MIT alternative selected; musl MIT and LLVM runtime license with exceptions | Browser C++ processing module/runtime |
| [miniaudio](https://github.com/mackron/miniaudio) | `f40cf03f80cdb7e741d43e53b7e706e8c1394bcf` (0.11.23) | MIT-0 alternative selected | WAV/FLAC/MP3 codecs and native device output |
| [Symphonia](https://github.com/pdeljanov/Symphonia) | `0.5.5`; exact transitive versions in `native/alac/Cargo.lock` | MPL-2.0; transitive dependencies retain their included notices | Windows Apple Lossless M4A decoding |
| [Rust](https://www.rust-lang.org/) | `1.90.0` in Windows installer CI; build inventory records the actual toolchain | MIT/Apache-2.0 and bundled third-party notices | Statically linked ALAC fallback runtime |
| [libav.js](https://github.com/Yahweasel/libav.js/releases/tag/v4.10.6.1.1) | npm `4.10.6`, build `4.10.6.1.1`, fat variant | LGPL-2.1-or-later for FFmpeg 6.1.1; ISC wrapper; dependencies retain their embedded notices | Browser M4A/AAC/ALAC and Ogg Opus decoding |
| [Windows App SDK / WinUI](https://github.com/microsoft/WindowsAppSDK) | NuGet `1.8.260209005` | Microsoft Software License Terms; component notices retained | Native Windows UI and deployment runtime |
| [.NET](https://github.com/dotnet/runtime) | .NET 10 SDK/runtime selected by the Windows build | MIT and bundled third-party notices | Self-contained Windows C# client |
| [Microsoft Visual C++ runtime](https://learn.microsoft.com/cpp/windows/redistributing-visual-cpp-files) | App-local x64 CRT from the Windows build tools | Microsoft redistributable terms | WinUI native runtime dependencies |

CMake downloads these revisions and installs their complete license texts into
`licenses/`. Distribute that directory with native binaries, together with this
file and Riffine's LICENSE. Build scripts and CI also copy the Go toolchain's
LICENSE and PATENTS. The CI artifact includes these notices. Release packaging
must add a complete dependency inventory.

Windows installers include the exact unmodified ALAC dependency sources and
license texts in `licenses/alac-sources/`, the Cargo manifest/lockfile needed to
rebuild them, and Rust's complete copyright notices in `licenses/rust-copyright.html`.
Riffine's ALAC adapter is MIT licensed and lives in `native/alac/src/lib.rs`.

The Windows build also collects the restored runtime packages' complete license
and notice files in `licenses/windows/`, including the selected package versions
in the folder names. The published app's Microsoft components retain their own
terms; Riffine's MIT license does not relicense them. Inno Setup builds the
installer; its compiled setup/uninstall engine is covered by
[Inno Setup's distribution license](https://jrsoftware.org/files/is/license.txt).

The browser DSP build copies complete Signalsmith, Emscripten, musl, libc++, libc++abi, libunwind and compiler-rt
notices into `web/public/practice/licenses/`, together with Riffine's license and
this inventory. Keep those files with the generated browser module and WASM.
The Emscripten notice includes its bundled runtime's additional components.

The media decoder is an independent, replaceable WASM module. The npm build
copies its complete embedded license inventory to `practice/media/LICENSES.txt`
and the upstream corresponding sources, dependency archives and build recipes
to `practice/media/sources/`. `SOURCES.txt` identifies the exact build and our
module adaptation script. Keep that entire directory with production web assets;
source archives are available alongside the decoder and are not loaded during
playback. No GPL-only FFmpeg configuration is used. The `fat` variant includes
additional unused audio codecs; their notices and corresponding sources are
included too. Riffine's own code remains MIT licensed.

Future Wails, FFT acceleration, codecs or DSP alternatives require their own
review. Do not silently add GPL/commercial DSP dependencies to an MIT-only build.
No rights to users' imported recordings are granted by this repository's license.

Riffine is a working name. Repository availability is not trademark clearance.
Perform appropriate trademark/name checks in target markets before branding a
release. This project is independent of Transcribe! and its publisher.
