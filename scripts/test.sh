#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
if [ "${1:-}" = "--native" ]; then
  export RIFFINE_AUDIO_HELPER="$(pwd)/bin/riffine-audio"
  test -x "$RIFFINE_AUDIO_HELPER"
  ctest --test-dir build/native -C Release --output-on-failure --no-tests=error
fi
go test -count=1 ./...
go vet ./...
test -z "$(gofmt -l adapters cmd core internal)"
