param([switch]$Native)
$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '..')
$previousHelper = $env:RIFFINE_AUDIO_HELPER
try {
    if ($Native) {
        $env:RIFFINE_AUDIO_HELPER = (Resolve-Path bin/riffine-audio.exe).Path
        ctest --test-dir build/native -C Release --output-on-failure --no-tests=error
        if ($LASTEXITCODE -ne 0) { throw 'Native streaming tests failed' }
    }
    go test -count=1 ./...
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed' }
    $unformatted = gofmt -l adapters cmd core internal
    if ($unformatted) { throw "Run gofmt on: $unformatted" }
} finally {
    $env:RIFFINE_AUDIO_HELPER = $previousHelper
    Pop-Location
}
