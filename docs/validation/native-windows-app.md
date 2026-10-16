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

The **Native Windows app** GitHub Actions workflow builds the same app and setup
program on Windows. Main/PR/manual runs retain the setup in the
`riffine-windows-x64-installer` artifact. Publish a Windows alpha by pushing an
explicit `windows-v0.1.0` tag after checking the build and acceptance items below;
the tag workflow attaches the setup and `SHA256SUMS.txt` to a GitHub prerelease.
Builds are unsigned until a code-signing identity is configured.

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
The macOS app build stops at Microsoft's Windows-only XAML compiler. The first
Windows CI run successfully compiled/published the complete WinUI client and
native helper; installer and native-window acceptance are tracked separately.

## Outstanding acceptance

- WinUI compilation and startup on a Windows runner, followed by clean-machine
  install, launch, upgrade and uninstall checks.
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
