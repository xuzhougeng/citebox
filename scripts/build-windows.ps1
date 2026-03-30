param(
    [Parameter(Mandatory = $true)]
    [string]$Version
)

$ErrorActionPreference = "Stop"

if ($env:OS -ne "Windows_NT") {
    throw "Windows server builds must run on native Windows so CGO and MuPDF stay enabled."
}

$binaryName = "citebox"
$outputDir = Join-Path "bin" "windows"
$outputPath = Join-Path $outputDir "$binaryName.exe"
$buildTime = Get-Date -AsUTC -Format "yyyy-MM-ddTHH:mm:ssZ"
$fitzLib = Join-Path "third_party\go-fitz\libs" "libmupdf_windows_amd64.a"
$buildTags = @()
$cgoEnabled = "1"

if ($env:CITEBOX_FORCE_NOCGO -eq "1") {
    Write-Host "CITEBOX_FORCE_NOCGO=1, building Windows server binary with -tags nocgo"
    $buildTags = @("-tags", "nocgo")
    $cgoEnabled = "0"
} elseif (-not (Test-Path $fitzLib)) {
    Write-Host "MuPDF static library not found at $fitzLib, building Windows server binary with -tags nocgo"
    $buildTags = @("-tags", "nocgo")
    $cgoEnabled = "0"
}

New-Item -ItemType Directory -Path $outputDir -Force | Out-Null

$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = $cgoEnabled

try {
    go build -trimpath @buildTags -ldflags "-s -w -X github.com/xuzhougeng/citebox/internal/buildinfo.Version=$Version -X github.com/xuzhougeng/citebox/internal/buildinfo.BuildTime=$buildTime" -o $outputPath ./cmd/server
} finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

Write-Host "Built $outputPath"
