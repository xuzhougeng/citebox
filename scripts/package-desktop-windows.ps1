param(
    [Parameter(Mandatory = $true)]
    [string]$Version
)

$ErrorActionPreference = "Stop"

$binaryName = "citebox-desktop"
$packageDir = Join-Path "dist" "$binaryName-windows-$Version"
$archivePath = "$packageDir.zip"
$hostArch = go env GOARCH

if (Test-Path $packageDir) {
    Remove-Item $packageDir -Recurse -Force
}

New-Item -ItemType Directory -Path $packageDir -Force | Out-Null

$env:CGO_ENABLED = "1"
$env:GOOS = "windows"
go build -trimpath -ldflags "-s -w -H windowsgui" -o (Join-Path $packageDir "$binaryName.exe") ./cmd/desktop
Remove-Item Env:GOOS
Remove-Item Env:CGO_ENABLED

Copy-Item "web" -Destination $packageDir -Recurse
Copy-Item "README.md" -Destination $packageDir
go run .\scripts\fetch_pdfjs.go (Join-Path $packageDir "web\static\vendor\pdfjs")

$startBat = @"
@echo off
chcp 65001 >nul
cd /d %~dp0
start "CiteBox Desktop" citebox-desktop.exe
"@

$readmeTxt = @"
CiteBox Desktop (Windows)
=========================

Contents:
- citebox-desktop.exe
- web\
- start.bat

Quick start:
1. Unzip the package.
2. Open the extracted folder.
3. Double-click start.bat.

Default account: citebox / citebox123
Binary architecture: $hostArch

Desktop mode stores data in:
- %AppData%\CiteBox\

Notes:
- This package expects the native GitHub Windows runner toolchain to compile cgo.
- The desktop app starts without a visible console window by default.
- WebView2 is required at runtime on Windows.
"@

Set-Content -Path (Join-Path $packageDir "start.bat") -Value $startBat -Encoding ascii
Set-Content -Path (Join-Path $packageDir "README.txt") -Value $readmeTxt -Encoding ascii

if (Test-Path $archivePath) {
    Remove-Item $archivePath -Force
}

Compress-Archive -Path $packageDir -DestinationPath $archivePath -CompressionLevel Optimal

Write-Host "Created $archivePath"
