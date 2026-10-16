#ifndef AppVersion
  #define AppVersion "0.1.0"
#endif
#ifndef PublishDir
  #error PublishDir must name the complete self-contained Windows publish folder
#endif
#ifndef InstallerDir
  #error InstallerDir must name the installer output folder
#endif

[Setup]
AppId={{26C2645D-712E-442D-8A22-BA2C2A5B52CF}
AppName=Riffine
AppVersion={#AppVersion}
AppPublisher=Riffine contributors
AppPublisherURL=https://github.com/NLstn/riffine
DefaultDirName={localappdata}\Programs\Riffine
DefaultGroupName=Riffine
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0.19041
DisableProgramGroupPage=yes
OutputDir={#InstallerDir}
OutputBaseFilename=Riffine-{#AppVersion}-windows-x64-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\Riffine.Windows.exe
CloseApplications=yes
RestartApplications=no

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Shortcuts:"; Flags: unchecked

[Files]
Source: "{#PublishDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\Riffine"; Filename: "{app}\Riffine.Windows.exe"
Name: "{autodesktop}\Riffine"; Filename: "{app}\Riffine.Windows.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\Riffine.Windows.exe"; Description: "Open Riffine"; Flags: nowait postinstall skipifsilent
