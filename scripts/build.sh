#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p bin
mkdir -p licenses
cp "$(go env GOROOT)/LICENSE" licenses/go-LICENSE
cp "$(go env GOROOT)/PATENTS" licenses/go-PATENTS
go build -trimpath -o bin/riffine ./cmd/riffine
if [ "${1:-}" = "--native" ]; then
  cmake -S native -B build/native -DCMAKE_BUILD_TYPE=Release
  cmake --build build/native --config Release
  cmake --install build/native --config Release --prefix .
fi
