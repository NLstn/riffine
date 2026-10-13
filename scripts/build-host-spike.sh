#!/usr/bin/env sh
# Builds feasibility artifacts only. No downloads, SDK installs or product deps.
set -eu
cd "$(dirname "$0")/.."
output="${RIFFINE_SPIKE_OUT:-build/host-spike}"
mkdir -p "$output"
output="$(cd "$output" && pwd)"
mode="${1:-portable}"
case "$mode" in
 portable)
  go build -trimpath -o "$output/host-spike" ./cmd/host-spike
  GOOS=js GOARCH=wasm go build -trimpath -o "$output/core.wasm" ./cmd/host-spike-wasm
  cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$output/wasm_exec.js"
  cp spikes/browser/index.html spikes/browser/worker.js "$output/"
  cp core/practice/testdata/session-v1.json "$output/fixture.json"
  ;;
 native)
  CGO_ENABLED=1 go build -trimpath -buildmode=c-archive -o "$output/libhost-spike.a" ./cmd/host-spike-mobile
  "${CC:-cc}" -I "$output" spikes/mobile/runner.c "$output/libhost-spike.a" -lpthread -lm -o "$output/mobile-runner"
  ;;
 ios)
  sdk="$(xcrun --sdk iphonesimulator --show-sdk-path)"
  clang="$(xcrun --sdk iphonesimulator --find clang)"
  # Use a simulator-targeted executable for launch readiness, not a macOS tool.
  "$clang" -target arm64-apple-ios14.0-simulator -isysroot "$sdk" spikes/mobile/spawn_probe.c -o "$output/spawn-probe"
  # arm64 simulator; device slice/signing and framework packaging are future work.
  CGO_ENABLED=1 GOOS=ios GOARCH=arm64 CC="$clang -target arm64-apple-ios14.0-simulator -isysroot $sdk" go build -trimpath -buildmode=c-archive -o "$output/libhost-spike.a" ./cmd/host-spike-mobile
  "$clang" -target arm64-apple-ios14.0-simulator -isysroot "$sdk" -I "$output" spikes/mobile/runner.c "$output/libhost-spike.a" -framework Foundation -framework CoreFoundation -lresolv -o "$output/mobile-runner"
  ;;
 android)
  : "${ANDROID_NDK_HOME:?Set ANDROID_NDK_HOME to an installed NDK with an arm64 API 24 compiler}"
  : "${RIFFINE_NDK_HOST_TAG:?Set RIFFINE_NDK_HOST_TAG to darwin-x86_64 or linux-x86_64}"
  arch="${RIFFINE_ANDROID_ARCH:-arm64}"
  case "$arch" in
   arm64) target=aarch64-linux-android24 ;;
   amd64) target=x86_64-linux-android24 ;;
   *) echo "RIFFINE_ANDROID_ARCH must be arm64 or amd64" >&2; exit 2 ;;
  esac
  clang="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/$RIFFINE_NDK_HOST_TAG/bin/$target-clang"
  test -x "$clang"
  CGO_ENABLED=1 GOOS=android GOARCH="$arch" CC="$clang" go build -trimpath -buildmode=c-shared -o "$output/libhost-spike.so" ./cmd/host-spike-mobile
  "$clang" -I "$output" spikes/mobile/runner.c -L "$output" -lhost-spike -o "$output/mobile-runner"
  ;;
 *) echo "Usage: $0 [portable|native|ios|android]" >&2; exit 2 ;;
esac
