#!/usr/bin/env sh
# Requires an already activated Emscripten SDK. Never installs a toolchain.
set -eu
cd "$(dirname "$0")/.."
command -v emcmake >/dev/null
emcmake cmake -S native/wasm -B build/browser-dsp -DCMAKE_BUILD_TYPE=Release
cmake --build build/browser-dsp --parallel 2
mkdir -p web/public/practice
cp build/browser-dsp/riffine-browser-dsp.mjs web/public/practice/dsp.mjs
cp build/browser-dsp/riffine-browser-dsp.wasm web/public/practice/riffine-browser-dsp.wasm

# Preserve complete notices with the generated static web assets.
emcc_path="$(command -v emcc)"
emscripten_source="$(dirname "$emcc_path")"
mkdir -p web/public/practice/licenses
cp build/browser-dsp/_deps/signalsmith-stretch-src/LICENSE.txt web/public/practice/licenses/signalsmith-stretch.txt
cp build/browser-dsp/_deps/signalsmith-linear-src/LICENSE.txt web/public/practice/licenses/signalsmith-linear.txt
cp "$emscripten_source/LICENSE" web/public/practice/licenses/emscripten.txt
cp "$emscripten_source/system/lib/libc/musl/COPYRIGHT" web/public/practice/licenses/musl.txt
cp "$emscripten_source/system/lib/libcxx/LICENSE.TXT" web/public/practice/licenses/libcxx.txt
cp LICENSE web/public/practice/licenses/riffine.txt
cp THIRD_PARTY_NOTICES.md web/public/practice/licenses/THIRD_PARTY_NOTICES.md
cp "$emscripten_source/system/lib/libcxxabi/LICENSE.TXT" web/public/practice/licenses/libcxxabi.txt
cp "$emscripten_source/system/lib/libunwind/LICENSE.TXT" web/public/practice/licenses/libunwind.txt
cp "$emscripten_source/system/lib/compiler-rt/LICENSE.TXT" web/public/practice/licenses/compiler-rt.txt
