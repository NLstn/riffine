# ADR 0006: Native Windows UI with a private Go control process

Status: accepted for the first Windows client. Date: 2026-10-16.

## Decision

Use C# and WinUI 3 for the Windows client. Reproduce the web client's dark
practice workspace, music library and green waveform using native controls and
source-coordinate drawing. The Windows client is a separate presentation host;
the existing React client continues to serve the browser.

Keep project validation, identity, command ordering, parameters, loop rules,
waveform analysis and persistence in Go. `riffine-desktop.exe` owns one
`adapters/binding.Session` using filesystem resources and the native decoder.
The UI exchanges bounded version-1 JSON messages on private redirected process
pipes. It opens no local HTTP server or network endpoint. Each operation has a
monotonic integer ID; one operation is active at a time. Cancellation targets
that ID and retains its final response, preserving pipe framing. Failed imports
and opens preserve the previous session. EOF cancels and joins the worker.

Decode, processing and audio output stay in the isolated C++ helper. Only
commands, snapshots and waveform extrema cross the UI bridge. The app reports
the core's capabilities and offers fixed-selection, single-pass native preview.
Continuous playback, live parameter edits, pause/seek and acknowledged repetition
counts require the remaining native presentation-aware device integration;
neither UI timers nor preview diagnostics may substitute for that work.

## Packaging

Target Windows 10 build 19041 or newer, x64 initially. Publish a self-contained
.NET/Windows App SDK app, its Go host, the native helper, required VC++ runtime
DLLs and notices in one folder. Inno Setup creates a per-user `.exe` installer
with Start menu/uninstall integration and an optional desktop shortcut.

The Windows workflow builds installers for PRs and main and retains downloads as
Actions artifacts. An explicit `windows-vMAJOR.MINOR.PATCH` tag creates a GitHub
prerelease containing the installer and SHA-256 checksum. Unsigned alpha builds
remain distinct from signed, supported releases under R6.

## Consequences

The Go and C++ layers remain shared, while React controls/styles are recreated in
WinUI. Windows compilation, install/uninstall, DPI/keyboard/accessibility checks
and actual WASAPI listening require Windows. macOS can build/cross-compile the
Go host and exercise the process protocol but cannot validate WinUI rendering.

References: [WinUI](https://learn.microsoft.com/windows/apps/winui),
[Windows deployment](https://learn.microsoft.com/windows/apps/package-and-deploy/),
[App SDK file pickers](https://learn.microsoft.com/windows/apps/develop/files/using-file-folder-pickers).
