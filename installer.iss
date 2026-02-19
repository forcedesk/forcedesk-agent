#define MyAppName      "ForceDesk Agent"
#define MyAppVersion   "1.0"
#define MyAppPublisher "ForceDesk"
#define MyAppExeName   "forcedesk-agent.exe"

[Setup]
AppId={{6F3A1D2E-4B5C-4E7F-8A9B-0C1D2E3F4A5B}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppUpdatesURL=https://github.com/forcedesk/forcedesk-agent
DefaultDirName={autopf}\{#MyAppName}
DisableProgramGroupPage=yes
DisableWelcomePage=no
OutputBaseFilename=forcedesk-agent-setup
OutputDir=.
SetupIconFile=forcedesk-icon.ico
Compression=lzma
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\{#MyAppExeName}
CloseApplications=no
RestartIfNeededByRun=no

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
; The agent exe is downloaded at install time — see PrepareToInstall below.
Source: "forcedesk-icon.ico"; DestDir: "{app}"; Flags: ignoreversion

[UninstallDelete]
; Remove the downloaded agent binary on uninstall (not tracked via [Files]).
Type: files; Name: "{app}\{#MyAppExeName}"

[Run]
; Install the Windows Service (auto-start, LocalSystem).
Filename: "{app}\{#MyAppExeName}"; Parameters: "install"; \
  StatusMsg: "Installing Windows service..."; \
  Flags: runhidden waituntilterminated

; Start the service immediately after installation.
Filename: "{app}\{#MyAppExeName}"; Parameters: "start"; \
  StatusMsg: "Starting ForceDesk Agent service..."; \
  Flags: runhidden waituntilterminated

[UninstallRun]
; Stop the service gracefully before removing files.
Filename: "{app}\{#MyAppExeName}"; Parameters: "stop"; \
  Flags: runhidden waituntilterminated skipifdoesntexist; \
  RunOnceId: "StopService"

; Remove the service registration.
Filename: "{app}\{#MyAppExeName}"; Parameters: "uninstall"; \
  Flags: runhidden waituntilterminated skipifdoesntexist; \
  RunOnceId: "UninstallService"

[Code]

const
  AgentDownloadURL = 'https://cdn.forcedesk.io/assets/forcedeskagent/latest.exe';

{ -----------------------------------------------------------------------
  Validate that config.toml is present next to the installer before
  allowing setup to proceed. Without it the agent cannot function and
  we treat the package as damaged/incomplete.
  ----------------------------------------------------------------------- }
function InitializeSetup(): Boolean;
begin
  Result := True;

  if not FileExists(ExpandConstant('{src}\config.toml')) then
  begin
    MsgBox(
      'This installer is damaged or incomplete.' + #13#10#13#10 +
      'The required file config.toml was not found next to the installer.' + #13#10 +
      'Please obtain a complete copy of the installer package.',
      mbCriticalError, MB_OK);
    Result := False;
  end;
end;

{ -----------------------------------------------------------------------
  Download the latest agent binary from the CDN before installation
  begins. Returning a non-empty string aborts setup and displays the
  message to the user.
  ----------------------------------------------------------------------- }
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  TmpExe:   String;
  PSCmd:    String;
  ExitCode: Integer;
begin
  Result := '';
  TmpExe := ExpandConstant('{tmp}\forcedesk-agent.exe');

  WizardForm.StatusLabel.Caption := 'Downloading ForceDesk Agent...';
  PSCmd := '-NoProfile -NonInteractive -Command ' +
           '"Invoke-WebRequest -Uri ''' + AgentDownloadURL + ''' ' +
           '-OutFile ''' + TmpExe + ''' -UseBasicParsing"';

  if not Exec('powershell.exe', PSCmd, '', SW_HIDE, ewWaitUntilTerminated, ExitCode)
     or (ExitCode <> 0) then
    Result := 'Failed to download ForceDesk Agent from the update server.' + #13#10#13#10 +
              'Please check your internet connection and try again.';
end;

{ -----------------------------------------------------------------------
  Copy config.toml into the data directory and move the downloaded
  agent binary into the install directory.
  ----------------------------------------------------------------------- }
procedure CurStepChanged(CurStep: TSetupStep);
var
  SrcConfig: string;
  DestDir:   string;
begin
  if CurStep = ssInstall then
  begin
    { Copy config.toml into the data directory }
    SrcConfig := ExpandConstant('{src}\config.toml');
    DestDir   := ExpandConstant('{commonappdata}\ForceDeskAgent');

    if not DirExists(DestDir) then
      ForceDirectories(DestDir);

    if not FileCopy(SrcConfig, DestDir + '\config.toml', False) then
      MsgBox('Warning: could not copy config.toml to ' + DestDir + '.' + #13#10 +
             'The service may not start correctly.', mbError, MB_OK);

    { Move the downloaded agent binary into the install directory }
    if not FileCopy(ExpandConstant('{tmp}\forcedesk-agent.exe'),
                    ExpandConstant('{app}\forcedesk-agent.exe'), False) then
      MsgBox('Warning: could not place forcedesk-agent.exe in ' +
             ExpandConstant('{app}') + '.' + #13#10 +
             'The service may not start correctly.', mbError, MB_OK);
  end;
end;

{ -----------------------------------------------------------------------
  After uninstall, offer to remove the data directory
  (%ProgramData%\ForceDeskAgent) which holds config.toml and logs.
  Answering No preserves the config for a future reinstall.
  ----------------------------------------------------------------------- }
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  DataDir: string;
begin
  if CurUninstallStep = usPostUninstall then
  begin
    DataDir := ExpandConstant('{commonappdata}\ForceDeskAgent');
    if DirExists(DataDir) then
    begin
      if MsgBox(
        'Remove agent data directory?' + #13#10#13#10 +
        DataDir + #13#10#13#10 +
        'This contains config.toml and log files. ' +
        'Choose No to keep the configuration for a future reinstall.',
        mbConfirmation, MB_YESNO) = IDYES then
      begin
        DelTree(DataDir, True, True, True);
      end;
    end;
  end;
end;
