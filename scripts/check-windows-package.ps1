param([Parameter(Mandatory)][string]$Setup)
$ErrorActionPreference = 'Stop'
if (-not $IsWindows) { throw 'Windows installer acceptance must run on Windows.' }
$setupPath = (Resolve-Path $Setup).Path
$repository = Split-Path $PSScriptRoot -Parent
$smokeDirectory = Join-Path $repository 'build/windows/smoke'
$installDirectory = Join-Path $smokeDirectory 'installed'
New-Item -ItemType Directory -Force $smokeDirectory | Out-Null
$install = Start-Process $setupPath -ArgumentList "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP- /NOICONS /DIR=`"$installDirectory`" /LOG=`"$smokeDirectory/install.log`"" -Wait -PassThru
if ($install.ExitCode -ne 0) { throw "Installer failed: $($install.ExitCode)" }
try {
    foreach ($name in @('Riffine.Windows.exe', 'Riffine.Windows.pri', 'riffine-desktop.exe', 'riffine-audio.exe', 'vcruntime140.dll', 'licenses')) {
        if (-not (Test-Path (Join-Path $installDirectory $name))) { throw "Installed package is missing $name" }
    }
    # Verify the installed Go host, helper handshake and C# protocol together.
    dotnet (Join-Path $repository 'build/windows/bridge-check/Riffine.BridgeCheck.dll') $installDirectory
    if ($LASTEXITCODE -ne 0) { throw 'Installed C# / Go bridge check failed' }
    $report = Join-Path $smokeDirectory 'native-window.json'
    Remove-Item $report -ErrorAction SilentlyContinue
    $app = Start-Process (Join-Path $installDirectory 'Riffine.Windows.exe') -ArgumentList "--smoke-report `"$report`"" -PassThru
    if (-not $app.WaitForExit(30000)) {
        $app.Kill($true)
        throw 'Native WinUI startup did not complete within 30 seconds'
    }
    if ($app.ExitCode -ne 0 -or -not (Test-Path $report)) {
        if (Test-Path "$report.error") { Get-Content "$report.error" | Write-Host }
        Get-WinEvent -FilterHashtable @{ LogName = 'Application'; StartTime = (Get-Date).AddMinutes(-2) } -ErrorAction SilentlyContinue |
            Where-Object { $_.Message -match 'Riffine|Microsoft.UI.Xaml|CoreCLR' } |
            Select-Object TimeCreated, ProviderName, Message | Format-List | Out-String |
            Tee-Object -FilePath (Join-Path $smokeDirectory 'startup-events.txt') | Write-Host
        throw "Native WinUI startup failed: $($app.ExitCode)"
    }
    $loaded = Get-Content $report -Raw | ConvertFrom-Json
    if ($loaded.winui -ne 'loaded') { throw 'Native window did not report a completed load' }
} finally {
    $uninstaller = Join-Path $installDirectory 'unins000.exe'
    if (Test-Path $uninstaller) {
        $uninstall = Start-Process $uninstaller -ArgumentList "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /LOG=`"$smokeDirectory/uninstall.log`"" -Wait -PassThru
        if ($uninstall.ExitCode -ne 0) { throw "Uninstall failed: $($uninstall.ExitCode)" }
    }
}
if (Test-Path (Join-Path $installDirectory 'Riffine.Windows.exe')) { throw 'Uninstall left the application executable behind' }
Write-Host 'Windows setup, installed C# / Go bridge, native WinUI startup and uninstall passed.'
