# Sign Windows binaries with SSL.com eSigner CodeSignTool.
# Secrets come from the environment. This script never prints them.
param(
    [Parameter(Mandatory = $true)]
    [ValidateCount(1, 100)]
    [string[]]$Path
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$SecretNames = @(
    "ESIGNER_CREDENTIAL_ID",
    "ESIGNER_USERNAME",
    "ESIGNER_PASSWORD",
    "ESIGNER_TOTP_SECRET",
    "ESIGNER_PIN"
)
$CodeSignToolVersion = "v1.3.2"
$CodeSignToolSha256 = "F14B1E1EF14BFA1FD00279C363AAB0DEBBF5DCFBA0E4BCDCE5D22BB771DE0E3A"
$CodeSignToolUrl = "https://github.com/SSLcom/CodeSignTool/releases/download/$CodeSignToolVersion/CodeSignTool-$CodeSignToolVersion.zip"
$Rfc3161TimestampUrl = "http://ts.ssl.com"
$ArchiveExtensions = @(".zip", ".7z", ".tar", ".gz", ".tgz")
$BinaryExtensions = @(".exe", ".dll", ".msi", ".sys", ".cab", ".ocx", ".efi")

function Fail {
    param([Parameter(Mandatory = $true)][string]$Message)
    [Console]::Error.WriteLine($Message)
    exit 1
}

function Get-EsignerSecretValue {
    param([Parameter(Mandatory = $true)][string]$Name)
    $value = [Environment]::GetEnvironmentVariable($Name)
    if ([string]::IsNullOrWhiteSpace($value)) {
        return $null
    }
    return $value.Trim()
}

function Hide-SecretValues {
    param(
        [string]$Text,
        [hashtable]$Secrets
    )
    if ([string]::IsNullOrEmpty($Text)) {
        return ""
    }
    foreach ($value in $Secrets.Values) {
        if ($value -and $value.Length -ge 4) {
            $Text = $Text.Replace($value, "***")
        }
    }
    return $Text
}

function Find-SignTool {
    $roots = @(
        (Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"),
        (Join-Path $env:ProgramFiles "Windows Kits\10\bin")
    )
    foreach ($root in $roots) {
        if (-not (Test-Path -LiteralPath $root)) {
            continue
        }
        $found = Get-ChildItem -Path $root -Filter "signtool.exe" -Recurse -File -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -match '\\x64\\signtool.exe$' } |
            Sort-Object FullName -Descending |
            Select-Object -First 1
        if ($found) {
            return $found.FullName
        }
    }
    return $null
}

function Assert-AuthenticodeSignature {
    param([Parameter(Mandatory = $true)][string]$File)
    $signature = Get-AuthenticodeSignature -LiteralPath $File
    if ($null -eq $signature.SignerCertificate) {
        Fail "$File has no Authenticode signer certificate."
    }
    if ($signature.Status -ne "Valid") {
        Fail "$File Authenticode status is $($signature.Status): $($signature.StatusMessage)"
    }
    if ($null -eq $signature.TimeStamperCertificate) {
        Fail "$File is signed but has no RFC3161 timestamp certificate."
    }

    $signTool = Find-SignTool
    if ($signTool) {
        $verify = & $signTool verify /pa /v $File 2>&1 | Out-String
        if ($LASTEXITCODE -ne 0) {
            Fail "signtool verify failed for ${File}: $verify"
        }
    }

    Write-Host "Authenticode verified: $File"
    Write-Host "  Signer: $($signature.SignerCertificate.Subject)"
    Write-Host "  Timestamp: $($signature.TimeStamperCertificate.Subject)"
}

function Install-CodeSignTool {
    $cacheRoot = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { [IO.Path]::GetTempPath() }
    $toolRoot = Join-Path $cacheRoot "CodeSignTool-$CodeSignToolVersion"
    $jar = Join-Path $toolRoot "jar\code_sign_tool-1.3.2.jar"
    $propertiesPath = Join-Path $toolRoot "conf\code_sign_tool.properties"
    if ((Test-Path -LiteralPath $jar) -and (Test-Path -LiteralPath $propertiesPath)) {
        return @{ Root = $toolRoot; Jar = $jar; Properties = $propertiesPath }
    }

    if (Test-Path -LiteralPath $toolRoot) {
        Remove-Item -LiteralPath $toolRoot -Recurse -Force
    }
    New-Item -ItemType Directory -Path $toolRoot -Force | Out-Null

    $zipPath = Join-Path $cacheRoot "CodeSignTool-$CodeSignToolVersion.zip"
    Invoke-WebRequest -Uri $CodeSignToolUrl -OutFile $zipPath -UseBasicParsing
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $zipPath).Hash.ToUpperInvariant()
    if ($hash -ne $CodeSignToolSha256) {
        Remove-Item -LiteralPath $zipPath -Force -ErrorAction SilentlyContinue
        Fail "CodeSignTool $CodeSignToolVersion checksum mismatch."
    }
    Expand-Archive -LiteralPath $zipPath -DestinationPath $toolRoot -Force
    Remove-Item -LiteralPath $zipPath -Force

    if (-not (Test-Path -LiteralPath $jar) -or -not (Test-Path -LiteralPath $propertiesPath)) {
        Fail "CodeSignTool archive did not contain the expected jar and conf/code_sign_tool.properties."
    }
    return @{ Root = $toolRoot; Jar = $jar; Properties = $propertiesPath }
}

foreach ($item in $Path) {
    if (-not (Test-Path -LiteralPath $item -PathType Leaf)) {
        Fail "Signable file not found: $item"
    }
    $extension = [IO.Path]::GetExtension($item).ToLowerInvariant()
    if ($ArchiveExtensions -contains $extension) {
        Fail "Refusing to Authenticode-sign archive '$item'. Sign the binaries, then package them."
    }
    if ($BinaryExtensions -notcontains $extension) {
        Fail "Refusing to Authenticode-sign '$item'. Expected a Windows binary ($($BinaryExtensions -join ', '))."
    }
}

$secrets = @{}
$missing = @()
foreach ($name in $SecretNames) {
    $secrets[$name] = Get-EsignerSecretValue $name
    if (-not $secrets[$name]) {
        $missing += $name
    }
}

$requireSigning = $env:CITEBOX_REQUIRE_WINDOWS_SIGNING -eq "1"
if ($missing.Count -gt 0) {
    if ($requireSigning -or $missing.Count -lt $SecretNames.Count) {
        Fail "Windows Authenticode signing needs $($SecretNames -join ', '). Missing: $($missing -join ', ')."
    }
    Write-Warning "eSigner secrets are not configured. Skipping Authenticode signing."
    exit 0
}

if ($env:OS -ne "Windows_NT") {
    Fail "Authenticode signing must run on Windows."
}
if (-not (Get-Command java -ErrorAction SilentlyContinue)) {
    Fail "Java is required to run SSL.com CodeSignTool."
}

$tool = Install-CodeSignTool
$properties = Get-Content -Raw -LiteralPath $tool.Properties
if ($properties -notmatch "(?m)^TSA_URL=$([regex]::Escape($Rfc3161TimestampUrl))\s*$") {
    Fail "CodeSignTool TSA_URL must be the SSL.com RFC3161 endpoint $Rfc3161TimestampUrl."
}

$help = & java -jar $tool.Jar sign --help 2>&1 | Out-String
$passPin = $help -match "(?m)^\s*-pin\b" -or $help -match "-pin="
if ($passPin) {
    Write-Host "CodeSignTool accepts -pin. ESIGNER_PIN will be passed to sign."
} else {
    Write-Host "CodeSignTool sign has no -pin flag. ESIGNER_PIN stays required and is not placed on the command line."
}

foreach ($item in $Path) {
    $leaf = [IO.Path]::GetFileName($item)
    $outputDir = Join-Path ([IO.Path]::GetTempPath()) ("citebox-signed-" + [guid]::NewGuid().ToString("n"))
    New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
    try {
        $arguments = @(
            "-jar", $tool.Jar,
            "sign",
            "-username=$($secrets.ESIGNER_USERNAME)",
            "-password=$($secrets.ESIGNER_PASSWORD)",
            "-credential_id=$($secrets.ESIGNER_CREDENTIAL_ID)",
            "-totp_secret=$($secrets.ESIGNER_TOTP_SECRET)",
            "-input_file_path=$item",
            "-output_dir_path=$outputDir"
        )
        if ($passPin) {
            $arguments += "-pin=$($secrets.ESIGNER_PIN)"
        }

        Write-Host "Signing $leaf with SSL.com eSigner CodeSignTool ($CodeSignToolVersion)"
        $signed = $false
        $lastOutput = ""
        $lastCode = 1
        foreach ($attempt in 1..4) {
            Push-Location $tool.Root
            try {
                $lastOutput = & java @arguments 2>&1 | Out-String
                $lastCode = $LASTEXITCODE
            } finally {
                Pop-Location
                $logDir = Join-Path $tool.Root "logs"
                if (Test-Path -LiteralPath $logDir) {
                    Remove-Item -LiteralPath $logDir -Recurse -Force -ErrorAction SilentlyContinue
                }
            }

            if ($lastCode -eq 0) {
                $signed = $true
                break
            }

            # eSigner rejects a reused TOTP. Parallel Windows jobs can mint the same code in one 30s window.
            if ($lastOutput -match "(?i)invalid otp" -and $attempt -lt 4) {
                Write-Host "eSigner rejected the one-time password (attempt $attempt/4). Waiting for the next TOTP window."
                Start-Sleep -Seconds 35
                continue
            }
            break
        }

        if (-not $signed) {
            Fail (Hide-SecretValues "CodeSignTool failed to sign $leaf (exit $lastCode). $lastOutput" $secrets)
        }

        $signedPath = Join-Path $outputDir $leaf
        if (-not (Test-Path -LiteralPath $signedPath)) {
            $candidates = @(Get-ChildItem -LiteralPath $outputDir -Recurse -File | Where-Object { $_.Name -eq $leaf })
            if ($candidates.Count -ne 1) {
                Fail "CodeSignTool did not write a signed copy of $leaf."
            }
            $signedPath = $candidates[0].FullName
        }

        Assert-AuthenticodeSignature $signedPath
        Move-Item -LiteralPath $signedPath -Destination $item -Force
    } finally {
        if (Test-Path -LiteralPath $outputDir) {
            Remove-Item -LiteralPath $outputDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

exit 0
