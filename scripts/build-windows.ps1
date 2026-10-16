param(
    [ValidatePattern('^\d+\.\d+\.\d+$')][string]$Version = '0.1.0',
    [switch]$Installer
)
$ErrorActionPreference = 'Stop'
if (-not $IsWindows) { throw 'WinUI builds require Windows, the .NET 10 SDK and Visual Studio C++ build tools.' }
Push-Location (Join-Path $PSScriptRoot '..')
try {
    $repository = (Get-Location).Path
    $publish = Join-Path $repository 'build/windows/publish'
    $nativeBuild = Join-Path $repository 'build/windows/native'
    New-Item -ItemType Directory -Force $publish | Out-Null
    # Static CRT for our native helper; WinUI runtime CRT is bundled below.
    cmake -S native -B $nativeBuild -A x64 -DBUILD_TESTING=OFF -DCMAKE_MSVC_RUNTIME_LIBRARY=MultiThreaded
    if ($LASTEXITCODE -ne 0) { throw 'Native configuration failed' }
    cmake --build $nativeBuild --config Release --parallel 2
    if ($LASTEXITCODE -ne 0) { throw 'Native build failed' }
    dotnet publish desktop/windows/Riffine.Windows.csproj -c Release -r win-x64 --self-contained true -p:Platform=x64 -p:RestoreLockedMode=true "-p:Version=$Version" -o $publish
    if ($LASTEXITCODE -ne 0) { throw 'WinUI publish failed' }
    go build -trimpath '-ldflags=-H=windowsgui' -o (Join-Path $publish 'riffine-desktop.exe') ./cmd/riffine-desktop
    if ($LASTEXITCODE -ne 0) { throw 'Desktop core build failed' }
    cmake --install $nativeBuild --config Release --prefix (Join-Path $repository 'build/windows/native-package')
    if ($LASTEXITCODE -ne 0) { throw 'Native package install failed' }
    Copy-Item build/windows/native-package/bin/riffine-audio.exe $publish
    $licenses = Join-Path $publish 'licenses'
    New-Item -ItemType Directory -Force $licenses | Out-Null
    Copy-Item build/windows/native-package/licenses/* $licenses
    Copy-Item LICENSE, THIRD_PARTY_NOTICES.md $publish
    Copy-Item docs/validation/native-windows-app.md (Join-Path $publish 'README-Windows.md')
    $goRoot = go env GOROOT
    if ($LASTEXITCODE -ne 0) { throw 'Go toolchain unavailable' }
    Copy-Item (Join-Path $goRoot 'LICENSE') (Join-Path $licenses 'go-LICENSE')
    Copy-Item (Join-Path $goRoot 'PATENTS') (Join-Path $licenses 'go-PATENTS')
    # Preserve the selected Microsoft package versions and their full notices.
    $assets = Get-Content desktop/windows/obj/project.assets.json -Raw | ConvertFrom-Json
    $packageRoots = @($assets.packageFolders.PSObject.Properties.Name)
    foreach ($library in $assets.libraries.PSObject.Properties) {
        if ($library.Value.type -ne 'package') { continue }
        foreach ($packageRoot in $packageRoots) {
            $package = Join-Path $packageRoot $library.Name.ToLowerInvariant()
            if (-not (Test-Path $package)) { continue }
            $notices = Get-ChildItem $package -File | Where-Object Name -Match '^(license|notice|third.?party)'
            if ($notices) {
                $destination = Join-Path $licenses ('windows/' + $library.Name)
                New-Item -ItemType Directory -Force $destination | Out-Null
                $notices | Copy-Item -Destination $destination
            }
            break
        }
    }
    foreach ($framework in $assets.project.frameworks.PSObject.Properties.Value) {
        foreach ($dependency in $framework.downloadDependencies) {
            if ($dependency.name -ne 'Microsoft.NETCore.App.Runtime.win-x64') { continue }
            $runtimeVersion = ($dependency.version.Trim('[', ']') -split ',')[0].Trim()
            $runtimeNotices = Join-Path $licenses "windows/dotnet-runtime/$runtimeVersion"
            New-Item -ItemType Directory -Force $runtimeNotices | Out-Null
            foreach ($packageRoot in $packageRoots) {
                $runtimeLicense = Join-Path $packageRoot "microsoft.netcore.app.runtime.win-x64/$runtimeVersion/LICENSE.TXT"
                if (Test-Path $runtimeLicense) { Copy-Item $runtimeLicense $runtimeNotices; break }
            }
            Invoke-WebRequest "https://raw.githubusercontent.com/dotnet/runtime/v$runtimeVersion/THIRD-PARTY-NOTICES.TXT" -OutFile (Join-Path $runtimeNotices 'THIRD-PARTY-NOTICES.TXT')
        }
    }

    # App-local redistributable DLLs let this install work without asking users
    # to install Visual Studio or a separate VC++ runtime.
    $vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio/Installer/vswhere.exe'
    $vsInstall = & $vswhere -latest -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
    if (-not $vsInstall) { throw 'Visual Studio C++ redistributable files were not found' }
    $redist = Get-ChildItem (Join-Path $vsInstall 'VC/Redist/MSVC') -Directory | Sort-Object Name -Descending | Select-Object -First 1
    $crt = Get-ChildItem (Join-Path $redist.FullName 'x64/Microsoft.VC*.CRT') -Directory | Select-Object -First 1
    if (-not $crt) { throw 'x64 Visual C++ runtime files were not found' }
    Copy-Item (Join-Path $crt.FullName '*.dll') $publish
    @(
        "Application: Riffine $Version"
        "Windows App SDK: 1.8.260209005 (complete graph in NuGet package notices)"
        "Visual C++ app-local CRT: $($redist.Name) / $($crt.Name)"
        "Go: $(go version)"
        ".NET SDK: $(dotnet --version)"
        ".NET runtime: $runtimeVersion"
    ) | Set-Content (Join-Path $licenses 'windows-build-inventory.txt') -Encoding utf8NoBOM

    if ($Installer) {
        $compiler = Join-Path ${env:ProgramFiles(x86)} 'Inno Setup 6/ISCC.exe'
        if (-not (Test-Path $compiler)) { throw 'Install Inno Setup 6 to build the Windows installer' }
        $installerDirectory = Join-Path $repository 'build/windows/installer'
        & $compiler "/DAppVersion=$Version" "/DPublishDir=$publish" "/DInstallerDir=$installerDirectory" desktop/windows/installer.iss
        if ($LASTEXITCODE -ne 0) { throw 'Windows installer build failed' }
        $setup = Get-ChildItem $installerDirectory -Filter "Riffine-$Version-windows-x64-setup.exe"
        $hash = (Get-FileHash $setup.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $($setup.Name)" | Set-Content (Join-Path $installerDirectory 'SHA256SUMS.txt') -Encoding utf8NoBOM
        Write-Host "Installer: $($setup.FullName)"
    }
    Write-Host "Native Windows app: $(Join-Path $publish 'Riffine.Windows.exe')"
} finally { Pop-Location }
