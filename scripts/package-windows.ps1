param(
    [Parameter(Mandatory = $true)]
    [string]$Version
)

$ErrorActionPreference = "Stop"

$binaryName = "citebox"
$packageDir = Join-Path "dist" "$binaryName-windows-$Version"
$archivePath = "$packageDir.zip"

if (Test-Path $packageDir) {
    Remove-Item $packageDir -Recurse -Force
}

New-Item -ItemType Directory -Path (Join-Path $packageDir "data\library\papers") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $packageDir "data\library\figures") -Force | Out-Null

$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -trimpath -ldflags "-s -w" -o (Join-Path $packageDir "$binaryName.exe") ./cmd/server
Remove-Item Env:GOOS
Remove-Item Env:GOARCH

Copy-Item "web" -Destination $packageDir -Recurse
Copy-Item "README.md" -Destination $packageDir

$startBat = @"
@echo off
chcp 65001 >nul
cd /d %~dp0
echo ========================================
echo   CiteBox
echo ========================================
echo.
echo Starting server...
echo Default URL: http://localhost:8080
echo Default account: wanglab / wanglab789
echo A browser window will open automatically.
echo Close the "CiteBox Server" window to stop the app.
echo.
start "CiteBox Server" citebox.exe
timeout /t 2 /nobreak >nul
start "" http://localhost:8080
"@

$configBat = @"
@echo off
chcp 65001 >nul
cd /d %~dp0
set SERVER_PORT=8080
set ADMIN_USERNAME=wanglab
set ADMIN_PASSWORD=wanglab789
rem set PDF_EXTRACTOR_URL=http://127.0.0.1:8000
rem set STORAGE_DIR=.\data\library
rem set DATABASE_PATH=.\data\library.db
echo Starting server with custom configuration...
echo Current port: %SERVER_PORT%
echo.
start "CiteBox Server" citebox.exe
timeout /t 2 /nobreak >nul
start "" http://localhost:%SERVER_PORT%
"@

$readmeTxt = @"
CiteBox Windows package
=======================

Contents:
- citebox.exe
- web\
- data\
- start.bat
- start-with-config.bat

Quick start:
1. Unzip the package.
2. Open the extracted folder.
3. Double-click start.bat.

Default URL: http://localhost:8080
Default account: wanglab / wanglab789
"@

Set-Content -Path (Join-Path $packageDir "start.bat") -Value $startBat -Encoding ascii
Set-Content -Path (Join-Path $packageDir "start-with-config.bat") -Value $configBat -Encoding ascii
Set-Content -Path (Join-Path $packageDir "README.txt") -Value $readmeTxt -Encoding ascii

if (Test-Path $archivePath) {
    Remove-Item $archivePath -Force
}

Compress-Archive -Path $packageDir -DestinationPath $archivePath -CompressionLevel Optimal

Write-Host "Created $archivePath"
