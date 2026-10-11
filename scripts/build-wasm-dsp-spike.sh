#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
# Use an already activated, pinned Emscripten SDK. Never install a toolchain here.
command -v emcmake >/dev/null
cmake -S spikes/wasm-dsp -B build/wasm-dsp-native -DCMAKE_BUILD_TYPE=Release
cmake --build build/wasm-dsp-native --config Release --parallel 2
emcmake cmake -S spikes/wasm-dsp -B build/wasm-dsp -DCMAKE_BUILD_TYPE=Release
cmake --build build/wasm-dsp --parallel 2
cp spikes/wasm-dsp/index.html spikes/wasm-dsp/browser-worker.mjs build/wasm-dsp/
node spikes/wasm-dsp/check.mjs
