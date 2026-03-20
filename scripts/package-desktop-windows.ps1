param(
    [Parameter(Mandatory = $true)]
    [string]$Version
)

$ErrorActionPreference = "Stop"

$binaryName = "citebox-desktop"
$stageDir = Join-Path "dist" "$binaryName-windows-$Version"
$payloadDir = Join-Path $stageDir "payload"
$supportDir = Join-Path $stageDir "build-support"
$installerPath = Join-Path "dist" "$binaryName-windows-$Version.exe"
$nsisScriptPath = Join-Path $supportDir "installer.nsi"
$iconPath = Join-Path $supportDir "installer.ico"
$hostArch = go env GOARCH
$buildTime = Get-Date -AsUTC -Format "yyyy-MM-ddTHH:mm:ssZ"

if (Test-Path $stageDir) {
    Remove-Item $stageDir -Recurse -Force
}
if (Test-Path $installerPath) {
    Remove-Item $installerPath -Force
}

New-Item -ItemType Directory -Path $payloadDir -Force | Out-Null
New-Item -ItemType Directory -Path $supportDir -Force | Out-Null

$makensis = Resolve-Makensis

$env:CGO_ENABLED = "1"
$env:GOOS = "windows"
go build -trimpath -ldflags "-s -w -H windowsgui -X github.com/xuzhougeng/citebox/internal/buildinfo.Version=$Version -X github.com/xuzhougeng/citebox/internal/buildinfo.BuildTime=$buildTime" -o (Join-Path $payloadDir "$binaryName.exe") ./cmd/desktop
Remove-Item Env:GOOS
Remove-Item Env:CGO_ENABLED

Copy-Item "web" -Destination $payloadDir -Recurse
Copy-Item "README.md" -Destination $payloadDir
go run .\scripts\fetch_pdfjs.go (Join-Path $payloadDir "web\static\vendor\pdfjs")
go run .\scripts\render_app_icon -ico $iconPath -size 256

$readmeTxt = @"
CiteBox Desktop (Windows)
=========================

Contents:
- citebox-desktop.exe
- web\
- README.txt

Quick start:
1. Run the installer.
2. Launch CiteBox from the Start Menu or desktop shortcut.

Default account: citebox / citebox123
Binary architecture: $hostArch

Desktop mode stores data in:
- %AppData%\CiteBox\

Notes:
- The desktop app starts without a visible console window by default.
- WebView2 is required at runtime on Windows.
- Uninstalling the app does not remove data stored under %AppData%\CiteBox\.
"@

Set-Content -Path (Join-Path $payloadDir "README.txt") -Value $readmeTxt -Encoding ascii

$resolvedPayloadDir = (Resolve-Path $payloadDir).Path
$resolvedInstallerPath = (Resolve-Path (Split-Path $installerPath -Parent)).Path + "\" + (Split-Path $installerPath -Leaf)
$resolvedIconPath = (Resolve-Path $iconPath).Path

$nsisTemplate = @'
Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"

Name "CiteBox"
OutFile "__OUTPUT_PATH__"
InstallDir "$LocalAppData\Programs\CiteBox"
InstallDirRegKey HKCU "Software\CiteBox" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma

!define MUI_ABORTWARNING
!define MUI_ICON "__ICON_PATH__"
!define MUI_UNICON "__ICON_PATH__"
!define MUI_FINISHPAGE_RUN "$INSTDIR\citebox-desktop.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Launch CiteBox"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Section "Install"
  SetOutPath "$INSTDIR"
  File /r "__PAYLOAD_DIR__\*"

  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateDirectory "$SMPROGRAMS\CiteBox"
  CreateShortcut "$SMPROGRAMS\CiteBox\CiteBox.lnk" "$INSTDIR\citebox-desktop.exe"
  CreateShortcut "$SMPROGRAMS\CiteBox\Uninstall CiteBox.lnk" "$INSTDIR\Uninstall.exe"
  CreateShortcut "$DESKTOP\CiteBox.lnk" "$INSTDIR\citebox-desktop.exe"

  WriteRegStr HKCU "Software\CiteBox" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CiteBox" "DisplayName" "CiteBox"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CiteBox" "DisplayVersion" "__VERSION__"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CiteBox" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CiteBox" "DisplayIcon" "$INSTDIR\citebox-desktop.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CiteBox" "UninstallString" "$\"$INSTDIR\Uninstall.exe$\""
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CiteBox" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CiteBox" "NoRepair" 1

  Call WarnIfWebView2Missing
SectionEnd

Section "Uninstall"
  Delete "$DESKTOP\CiteBox.lnk"
  Delete "$SMPROGRAMS\CiteBox\CiteBox.lnk"
  Delete "$SMPROGRAMS\CiteBox\Uninstall CiteBox.lnk"
  RMDir "$SMPROGRAMS\CiteBox"

  Delete "$INSTDIR\Uninstall.exe"
  Delete "$INSTDIR\citebox-desktop.exe"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\README.txt"
  RMDir /r "$INSTDIR\web"
  RMDir /r "$INSTDIR"

  DeleteRegKey HKCU "Software\CiteBox"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CiteBox"
SectionEnd

Function WarnIfWebView2Missing
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 == ""
    ReadRegStr $0 HKLM "SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${EndIf}
  ${If} $0 == ""
    ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${EndIf}
  ${If} $0 == ""
    MessageBox MB_ICONEXCLAMATION|MB_OK "CiteBox requires Microsoft Edge WebView2 Runtime. Install it if the app does not open: https://go.microsoft.com/fwlink/p/?LinkId=2124703"
  ${EndIf}
FunctionEnd
'@

$nsisScript = $nsisTemplate.Replace("__OUTPUT_PATH__", $resolvedInstallerPath)
$nsisScript = $nsisScript.Replace("__ICON_PATH__", $resolvedIconPath)
$nsisScript = $nsisScript.Replace("__PAYLOAD_DIR__", $resolvedPayloadDir)
$nsisScript = $nsisScript.Replace("__VERSION__", $Version)

Set-Content -Path $nsisScriptPath -Value $nsisScript -Encoding ascii

& $makensis $nsisScriptPath | Write-Host

Write-Host "Created $installerPath"

function Resolve-Makensis {
    $command = Get-Command "makensis" -CommandType Application -ErrorAction SilentlyContinue
    if ($null -ne $command) {
        return $command.Source
    }

    $candidatePaths = @(
        (Join-Path $env:ProgramFiles "NSIS\makensis.exe"),
        (Join-Path ${env:ProgramFiles(x86)} "NSIS\makensis.exe")
    )

    if ($env:ChocolateyInstall) {
        $candidatePaths += Get-ChildItem `
            -Path (Join-Path $env:ChocolateyInstall "lib") `
            -Filter "makensis.exe" `
            -File `
            -Recurse `
            -ErrorAction SilentlyContinue |
            Select-Object -ExpandProperty FullName
    }

    foreach ($path in $candidatePaths) {
        if ($path -and (Test-Path $path)) {
            return (Resolve-Path $path).Path
        }
    }

    throw "makensis not found. Searched PATH, Program Files, and Chocolatey install directories."
}
