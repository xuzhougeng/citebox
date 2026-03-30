param(
    [string]$GoOS,
    [string]$GoArch,
    [switch]$Force
)

$ErrorActionPreference = "Stop"

if (-not $GoOS) {
    $GoOS = (go env GOOS).Trim()
}

if (-not $GoArch) {
    $GoArch = (go env GOARCH).Trim()
}

switch ($GoOS) {
    "linux" {
        if ($GoArch -notin @("amd64", "arm64")) {
            throw "unsupported GOARCH for linux: $GoArch"
        }
    }
    "darwin" {
        if ($GoArch -notin @("amd64", "arm64")) {
            throw "unsupported GOARCH for darwin: $GoArch"
        }
    }
    "windows" {
        if ($GoArch -ne "amd64") {
            throw "unsupported GOARCH for windows: $GoArch"
        }
    }
    default {
        throw "unsupported GOOS: $GoOS"
    }
}

$fitzGoFile = Join-Path $PSScriptRoot "..\third_party\go-fitz\fitz.go"
$fitzGoFile = (Resolve-Path $fitzGoFile).Path
$fitzGoContent = Get-Content -Path $fitzGoFile -Raw

if ($fitzGoContent -notmatch 'var FzVersion = "([^"]+)"') {
    throw "failed to detect go-fitz version from $fitzGoFile"
}

$fitzVersion = $matches[1]
$fitzTag = "v$fitzVersion"
$libsDir = Join-Path $PSScriptRoot "..\third_party\go-fitz\libs"
New-Item -ItemType Directory -Path $libsDir -Force | Out-Null

$files = @(
    "libmupdf_${GoOS}_${GoArch}.a",
    "libmupdfthird_${GoOS}_${GoArch}.a"
)

foreach ($fileName in $files) {
    $destination = Join-Path $libsDir $fileName
    if ((-not $Force) -and (Test-Path $destination)) {
        Write-Host "go-fitz lib already present: $destination"
        continue
    }

    $url = "https://raw.githubusercontent.com/gen2brain/go-fitz/$fitzTag/libs/$fileName"
    Write-Host "Downloading $fileName from gen2brain/go-fitz $fitzTag"
    Invoke-WebRequest -Uri $url -OutFile $destination

    $item = Get-Item $destination
    if ($item.Length -le 0) {
        throw "downloaded file is missing or empty: $destination"
    }
}

Write-Host "Prepared go-fitz libs for $GoOS/$GoArch from $fitzTag"
