# Native Windows app — first delivery slice

The client in `desktop/windows` uses native WinUI 3 controls and C# with the
existing Go practice core and native C++ audio helper. It targets Windows 10
build 19041 or later and Windows 11 on x64.

## Included

- Native file/folder dialogs and a searchable recursive WAV/FLAC/MP3 file list.
  The list shows filenames; source format/identity is checked by Go on open.
  At most 500 matching rows are displayed; search covers the full bounded list.
- Source waveform overview and zoom to the selected phrase, using the shared
  persistent waveform cache. Drag to select A/B or enter exact source seconds.
- Independent speed/pitch and phrase settings validated by the shared core.
- Single-pass native streaming preview and Stop/cancellation. Settings apply
  between previews. There is no fabricated playhead or repetition counter.
- Open portable projects, save-as with exclusive creation, revision-protected
  updates and recovery copies through the existing filesystem adapter.
- Recent training projects, unsaved-change prompts, a private cancellable Go
  control process and native-worker teardown on app close.
- A self-contained app folder, per-user setup executable and SHA-256 checksum.

Native format support currently covers PCM WAV, FLAC and MP3. Browser M4A/Opus
support does not establish native decoder support. Projects keep source audio
references; they do not copy or upload music. Sources must remain stable while
they are being analyzed or previewed.

## Build on Windows

Install .NET 10 SDK, Go 1.24+, CMake 3.24+, Git, and Visual Studio 2022 C++ build
tools or newer (Desktop development with C++ and a Windows SDK). Inno Setup 6 is needed
only for the installer. Run from PowerShell 7:

```powershell
./scripts/build-windows.ps1
./build/windows/publish/Riffine.Windows.exe
# Build the downloadable installer as well:
./scripts/build-windows.ps1 -Version 0.1.0 -Installer
```

The output folder contains .NET/Windows App SDK and VC++ runtime dependencies;
users install the complete app through the setup program. Native helper
processes run without opening console windows. No external browser or Node.js
installation is required.

## GitHub downloads and release channels

After a relevant merge to `main`, the **Native Windows app** workflow builds,
installs, launches and uninstalls the app on a Windows runner. Only a successful
run updates the rolling [Windows Nightly prerelease](https://github.com/NLstn/riffine/releases/tag/windows-nightly).
The first relevant merge creates this release. Download its
[Windows x64 installer](https://github.com/NLstn/riffine/releases/download/windows-nightly/Riffine-nightly-windows-x64-setup.exe)
or `SHA256SUMS.txt`; these URLs stay fixed across nightly updates. The release
notes identify the exact source commit and acceptance run. A failed build leaves
the last successful nightly available. Rerunning an older commit cannot replace
a newer nightly. "Nightly" means a build after every
relevant merge, rather than a scheduled daily build.

Relevant changes include the Windows UI and bridge check, desktop Go host,
shared core and desktop adapters, native engine (excluding its WASM-only build),
Go dependencies, bundled notices/guide, and Windows build/publication scripts
and workflow. Changes only to `desktop/macos/`, `web/`, mobile hosts or unrelated
documentation do not trigger a Windows build. Shared engine/core changes do.
Keep both event path lists in `.github/workflows/windows.yml` in sync when
adding Windows dependencies. GitHub applies these path filters to branch and PR
events; version tag pushes always build.

PR and manual runs retain downloads in the workflow's
`riffine-windows-x64-installer` artifact and never update a release. Manual runs
can choose an installer version. To publish a versioned build later, tag the
selected commit and push the explicit Windows tag:

```sh
git tag windows-v0.2.0 <commit>
git push origin windows-v0.2.0
```

The tag workflow builds that commit and attaches `Riffine-0.2.0-windows-x64-setup.exe`
and `SHA256SUMS.txt` to its own GitHub prerelease. Existing versioned downloads
are never overwritten. Version tags use `windows-vMAJOR.MINOR.PATCH`; keep them
separate from the moving `windows-nightly` tag. Versioned builds remain marked as
prereleases during alpha; promote a selected release in GitHub when the remaining
manual and supported-release acceptance gates pass. Builds are unsigned until a
code-signing identity is configured.

## Protocol and evidence

The UI launches `riffine-desktop.exe` beside itself with private stdin/stdout
pipes. Every message is one UTF-8 JSON line. Example:

```json
{"version":1,"id":1,"request":{"version":1,"type":"hello"}}
{"version":1,"id":2,"request":{"version":1,"type":"import","source":"C:\\Music\\song.wav","document":"C:\\Users\\me\\AppData\\Local\\Riffine\\Unsaved\\session.json"}}
{"version":1,"id":2,"cancel":true}
```

IDs for operations strictly increase; a cancel envelope targets the active ID
and adds no reply. The original operation always returns one final reply before
the next operation. Requests use the existing bounded session contract; response
lines are capped at 8 MiB. EOF/parent close cancels and joins the active operation
before releasing the session. Protocol failures terminate the private host.

Meaningful host checks cover import/edit/waveform/save/reopen through real
filesystem resources, failed replacement preserving the active project,
exclusive save behavior, cancellation followed by another operation, EOF joining
the worker, and malformed/oversized envelopes. Go checks and Windows compilation
are separate evidence from native UI/hardware acceptance.

Local macOS evidence for this slice: the full Go suite and the affected Go race
checks pass; the desktop host cross-compiles for Windows x64. The actual C#
client-to-Go process check passes for handshake, Unicode-path import, editing,
already-cancelled requests, failed replacement, waveform queries, exclusive save
and reopening. All C# sources type-check against the restored WinUI references.
The macOS app build stops at Microsoft's Windows-only XAML compiler. The
[Windows acceptance run](https://github.com/NLstn/riffine/actions/runs/38937983576)
compiled the complete WinUI app, built and installed its setup executable,
verified the installed C#↔Go bridge and actual native helper capabilities
(WAV/FLAC/MP3 and streaming preview), loaded a native WinUI window, closed it and
uninstalled successfully. The application PRI is explicitly published because
the SDK's unpackaged publish path omits it; both build and installed-package
checks require that resource index. See
[Windows App SDK issue 6720](https://github.com/microsoft/WindowsAppSDK/issues/6720).
Local native-backed validation also passes all 11 processing checks, the full Go
suite and `go vet` after rebuilding the helper.

## Outstanding acceptance

- Upgrade acceptance across future installer versions; fresh install, startup,
  close and uninstall are covered by the Windows runner.
- Native file picker behavior, unsaved-change prompts, project revision conflicts,
  Unicode/long paths and source relinking workflow on Windows.
- DPI scaling, narrow-window layout, keyboard/screen-reader use and waveform
  pointer behavior on Windows.
- Actual WASAPI preview/Stop and app-close behavior, including missing devices.
- Native continuous transport with pause/seek, live changes, repeated loops,
  practice cues and device presentation acknowledgements.
- Native M4A/Opus support, embedded tag/artwork parity, richer annotation controls,
  volume/mute and the remaining web/native product parity work.
- Installer/application signing and the remaining R6 supported-release gates.

This initial client does not close the daily-desktop or release gates. The
remaining native device work must preserve the shared transport contracts.
