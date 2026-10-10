param([switch]$Native)
$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
    New-Item -ItemType Directory -Force bin | Out-Null
    New-Item -ItemType Directory -Force licenses | Out-Null
    $goRoot = go env GOROOT
    if ($LASTEXITCODE -ne 0) { throw 'Go toolchain unavailable' }
    Copy-Item (Join-Path $goRoot 'LICENSE') licenses/go-LICENSE
    Copy-Item (Join-Path $goRoot 'PATENTS') licenses/go-PATENTS
    go build -trimpath -o bin/riffine.exe ./cmd/riffine
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
    if ($Native) {
        cmake -S native -B build/native
        if ($LASTEXITCODE -ne 0) { throw 'Native configuration failed' }
        cmake --build build/native --config Release
        if ($LASTEXITCODE -ne 0) { throw 'Native build failed' }
        cmake --install build/native --config Release --prefix .
        if ($LASTEXITCODE -ne 0) { throw 'Native install failed' }
    }
} finally { Pop-Location }
