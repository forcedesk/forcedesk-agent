; =============================================================================
;  ForceDesk Agent — NSIS installer
;  Translated from Inno Setup (.iss) source.
;
;  Build with:  makensis forcedesk-agent-setup.nsi
;  Requires NSIS 3.x with the MUI2 (Modern UI) header, which ships with all
;  standard NSIS installs (Homebrew, apt, the official Windows installer).
; =============================================================================

!include "MUI2.nsh"
!include "x64.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"

; -----------------------------------------------------------------------------
; App metadata  (equivalent to Inno's #define block + [Setup] basics)
; -----------------------------------------------------------------------------
!define MyAppName        "ForceDesk Agent"
!define MyAppVersion      "1.0"
!define MyAppPublisher    "ForceDesk"
!define MyAppExeName      "forcedesk-agent.exe"
!define MyAppUpdatesURL   "https://github.com/forcedesk/forcedesk-agent"
!define ServiceName       "ForceDeskAgent"

; Used for the Windows "Programs and Features" registry entry.
; Equivalent role to Inno's AppId GUID, just exposed differently.
!define UninstRegKey "Software\Microsoft\Windows\CurrentVersion\Uninstall\${ServiceName}"

Name "${MyAppName}"
OutFile "forcedesk-agent-setup.exe"
Unicode True

; Inno: DefaultDirName={autopf}\{#MyAppName}
InstallDir "$PROGRAMFILES64\${MyAppName}"

; Inno: PrivilegesRequired=admin
RequestExecutionLevel admin

; Inno: Compression=lzma / SolidCompression=yes
SetCompressor /SOLID lzma

; Inno: SetupIconFile / UninstallDisplayIcon
Icon "forcedesk-icon.ico"
UninstallIcon "forcedesk-icon.ico"

; -----------------------------------------------------------------------------
; Architecture check
; Inno: ArchitecturesAllowed=x64compatible / ArchitecturesInstallIn64BitMode=x64compatible
; NSIS has no manifest-level equivalent — enforce at runtime in .onInit instead.
; -----------------------------------------------------------------------------

; -----------------------------------------------------------------------------
; Modern UI configuration
;
; NOTE ON WizardStyle=modern dark polar includetitlebar:
; NSIS/MUI2 does not have a built-in dark-mode wizard skin. The closest
; faithful equivalent is MUI2's standard "modern" look with a custom
; sidebar/header bitmap (mirroring WizardImageFile/WizardSmallImageFile
; below). True dark-mode chrome would require a third-party UI patch
; (e.g. NSIS dark-mode forks) — flagging this rather than silently
; dropping the requirement, since it's a visible difference from the
; Inno build.
; -----------------------------------------------------------------------------
!define MUI_ICON "forcedesk-icon.ico"
!define MUI_UNICON "forcedesk-icon.ico"

; Inno: WizardImageFile (sidebar, large bitmap)
!define MUI_WELCOMEFINISHPAGE_BITMAP "installer-sidebar.bmp"
!define MUI_UNWELCOMEFINISHPAGE_BITMAP "installer-sidebar.bmp"

; Inno: WizardSmallImageFile (header, small bitmap)
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_BITMAP "installer-header.bmp"

; Inno: DisableWelcomePage=no -> we keep the welcome page (default MUI behaviour)
!insertmacro MUI_PAGE_WELCOME

; Inno: LicenseFile=license.txt
!insertmacro MUI_PAGE_LICENSE "license.txt"

; Inno: DisableProgramGroupPage=yes -> there is no Start Menu group page at all,
; so we simply omit MUI_PAGE_STARTMENU / MUI_PAGE_COMPONENTS.
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

!insertmacro MUI_LANGUAGE "English"

; -----------------------------------------------------------------------------
; .onInit — equivalent to Inno's InitializeSetup()
;
; Inno validated that config.toml sits next to the installer before allowing
; setup to proceed, and aborted with a "damaged/incomplete" message if not.
; We replicate that exactly, plus add the x64 check that Inno handled via
; ArchitecturesAllowed (NSIS needs this done manually).
; -----------------------------------------------------------------------------
Function .onInit

  ; Architecture gate — Inno: ArchitecturesAllowed=x64compatible
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "This installer requires a 64-bit version of Windows."
    Abort
  ${EndIf}

  ; config.toml presence check — Inno: InitializeSetup()
  ${IfNot} ${FileExists} "$EXEDIR\config.toml"
    MessageBox MB_ICONSTOP "This installer is damaged or incomplete.$\r$\n$\r$\n\
      The required file config.toml was not found next to the installer.$\r$\n\
      Please obtain a complete copy of the installer package."
    Abort
  ${EndIf}

FunctionEnd

; -----------------------------------------------------------------------------
; Main install section
; Inno [Files] -> File instructions
; Inno [Run]   -> ExecWait calls, run in the same order as the original
; Inno CurStepChanged(ssInstall) config copy -> placed before service install,
;   matching the original ordering (config copied during ssInstall, which
;   fires before [Run] entries execute)
; -----------------------------------------------------------------------------
Section "Install" SEC_MAIN

  ; =====================================================================
  ; FIRST ACTION OF THE INSTALL SECTION — runs immediately once the user
  ; has clicked through Welcome / License / Directory and the wizard
  ; begins the actual install. Everything else in this section, including
  ; shell context setup and file copying, happens after this.
  ;
  ; Stop the ForceDesk Agent service/process before touching anything else.
  ; This must happen before the [Files] copy further down, since the
  ; running service may hold a lock on forcedesk-agent.exe (and possibly
  ; cygwin1.dll / fping.exe) that would block overwriting them. Try a
  ; graceful service stop first; if the process is still present
  ; afterward (service stop can return before the process actually
  ; exits, or the process may be running outside the SCM's control),
  ; forcibly kill it by image name as a fallback.
  ; =====================================================================
  DetailPrint "Stopping ForceDesk Agent service..."
  nsExec::ExecToLog 'sc.exe stop ${ServiceName}'
  Pop $1

  ; Give the SCM a moment to actually tear down the process before checking.
  Sleep 2000

  nsExec::ExecToLog 'taskkill /F /IM "${MyAppExeName}"'
  Pop $1
  ; Exit code 128 from taskkill just means "process not found" — not an error
  ; in this context, since the graceful stop above may have already exited it.

  ; ---------------------------------------------------------------------
  ; Everything below here runs after the service-stop guard above.
  ; ---------------------------------------------------------------------

  ; Service runs as LocalSystem and installs machine-wide (PrivilegesRequired=admin
  ; in the original Inno script) — use the all-users shell context consistently
  ; for every path resolved in this section.
  SetShellVarContext all

  ; ---- Force unconditional overwrite of existing files -----------------------
  ; Explicit rather than relying on NSIS's default — this install must always
  ; replace existing files on disk (equivalent to Inno's Flags: ignoreversion,
  ; which skips version checks and always overwrites).
  SetOverwrite on

  SetOutPath "$INSTDIR"

  ; ---- [Files] block -------------------------------------------------------
  File "forcedesk-agent.exe"
  File "forcedesk-icon.ico"
  File "print_label.vbs"
  File "fping.exe"
  File "cygwin1.dll"

  SetOutPath "$INSTDIR\assets"
  File /r "assets\*.*"

  SetOutPath "$INSTDIR"
  File /r "lib\rrdtool\*.*"
  File "lib\bpacclient.msi"
  File "lib\brothersdk.exe"

  ; ---- Config copy (Inno: CurStepChanged / ssInstall) -----------------------
  ; Copies config.toml into %ProgramData%\ForceDeskAgent before the service
  ; is installed/started, same ordering as the Inno script.
  ;
  ; NOTE: NSIS has no built-in $PROGRAMDATA constant (unlike Inno's
  ; {commonappdata}). SetShellVarContext all + $APPDATA correctly resolves
  ; to %ProgramData% (C:\ProgramData) rather than the per-user
  ; %AppData%\Roaming path — this is the standard NSIS idiom for it.
  StrCpy $0 "$APPDATA\ForceDeskAgent"
  CreateDirectory "$0"
  CopyFiles /SILENT "$EXEDIR\config.toml" "$0\config.toml"
  IfErrors 0 +2
    MessageBox MB_ICONEXCLAMATION "Warning: could not copy config.toml to $0.$\r$\n\
      The service may not start correctly."

  ; ---- [Run] block: install + start the Windows service --------------------
  DetailPrint "Installing Windows service..."
  nsExec::ExecToLog '"$INSTDIR\${MyAppExeName}" install'
  Pop $1

  DetailPrint "Starting ForceDesk Agent service..."
  nsExec::ExecToLog '"$INSTDIR\${MyAppExeName}" start'
  Pop $1

  ; ---- Service verification (Inno: CurStepChanged / ssDone) -----------------
  ; "sc query" returns 0 only when the service is in the RUNNING state.
  nsExec::ExecToLog 'sc.exe query ${ServiceName}'
  Pop $1
  ${If} $1 != 0
    ; One more attempt to start the service
    nsExec::ExecToLog 'sc.exe start ${ServiceName}'
    Pop $1
    ${If} $1 != 0
      MessageBox MB_ICONEXCLAMATION "The ForceDesk Agent service was installed but could not be started.$\r$\n$\r$\n\
        You can start it manually from the Windows Services console (services.msc)."
    ${EndIf}
  ${EndIf}

  ; ---- Run bundled sub-installers from the app's base directory -------------
  ; bpacclient.msi and brothersdk.exe were previously only copied into place
  ; (matching the original Inno script, which also never executed them) —
  ; this runs them as the final install step, before the uninstaller is
  ; registered, so a failure here still leaves "finishing" logically last.

  MessageBox MB_OK|MB_ICONINFORMATION "Installing Brother Libraries for use with Remote Label Printing..."

  DetailPrint "Installing Brother P-touch Editor Lib (bpacclient.msi)..."
  ; MSIs must be invoked via msiexec, not executed directly. No silent flags —
  ; this will show the MSI's own UI.
  nsExec::ExecToLog 'msiexec.exe /i "$INSTDIR\bpacclient.msi"'
  Pop $1
  ${If} $1 != 0
    MessageBox MB_ICONEXCLAMATION "bpacclient.msi installation returned exit code $1."
  ${EndIf}

  DetailPrint "Installing Brother SDK (brothersdk.exe)..."
  ; No silent flags — this will show the vendor's own installer UI.
  nsExec::ExecToLog '"$INSTDIR\brothersdk.exe"'
  Pop $1
  ${If} $1 != 0
    MessageBox MB_ICONEXCLAMATION "brothersdk.exe installation returned exit code $1."
  ${EndIf}

  ; ---- Uninstaller registration (Programs and Features entry) --------------
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "${UninstRegKey}" "DisplayName" "${MyAppName}"
  WriteRegStr HKLM "${UninstRegKey}" "UninstallString" "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "${UninstRegKey}" "DisplayIcon" "$INSTDIR\${MyAppExeName}"
  WriteRegStr HKLM "${UninstRegKey}" "Publisher" "${MyAppPublisher}"
  WriteRegStr HKLM "${UninstRegKey}" "DisplayVersion" "${MyAppVersion}"
  WriteRegStr HKLM "${UninstRegKey}" "URLUpdateInfo" "${MyAppUpdatesURL}"

SectionEnd

; -----------------------------------------------------------------------------
; Uninstaller
; Inno [UninstallRun] -> stop + uninstall service, run BEFORE file removal
; Inno CurUninstallStepChanged(usPostUninstall) -> data dir prompt, run AFTER
;   files are gone, matching original ordering
; -----------------------------------------------------------------------------
Section "Uninstall"

  SetShellVarContext all

  ; ---- [UninstallRun]: stop then unregister the service ---------------------
  DetailPrint "Stopping ForceDesk Agent service..."
  nsExec::ExecToLog '"$INSTDIR\${MyAppExeName}" stop'
  Pop $1

  DetailPrint "Removing service registration..."
  nsExec::ExecToLog '"$INSTDIR\${MyAppExeName}" uninstall'
  Pop $1

  ; ---- Remove installed files ------------------------------------------------
  Delete "$INSTDIR\${MyAppExeName}"
  Delete "$INSTDIR\forcedesk-icon.ico"
  Delete "$INSTDIR\print_label.vbs"
  Delete "$INSTDIR\fping.exe"
  Delete "$INSTDIR\cygwin1.dll"
  Delete "$INSTDIR\bpacclient.msi"
  Delete "$INSTDIR\brothersdk.exe"
  RMDir /r "$INSTDIR\assets"
  RMDir /r "$INSTDIR"

  DeleteRegKey HKLM "${UninstRegKey}"

  ; ---- Post-uninstall data dir prompt (Inno: usPostUninstall) ---------------
  StrCpy $0 "$APPDATA\ForceDeskAgent"
  ${If} ${FileExists} "$0\*.*"
    MessageBox MB_YESNO|MB_ICONQUESTION "Remove agent data directory?$\r$\n$\r$\n\
      $0$\r$\n$\r$\n\
      This contains config.toml and log files. \
      Choose No to keep the configuration for a future reinstall." IDYES removeData IDNO keepData
    removeData:
      RMDir /r "$0"
    keepData:
  ${EndIf}

SectionEnd
