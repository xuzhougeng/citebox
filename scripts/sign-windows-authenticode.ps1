# Sign Windows binaries with SSL.com eSigner CodeSignTool.
# Secrets come from the environment. This script never prints them.
#
# CodeSignTool v1.3.2 main() discards picocli's exit code, so `java -jar`
# exits 0 when signing fails and the reason is only on stdout/stderr
# (`Error: ...`) or in logs/code_signing_tool.log. Success requires exit 0,
# a signed file, and "Code signed successfully:".
param(
    [ValidateCount(1, 100)]
    [string[]]$Path
)

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
$CodeSignTimeoutSeconds = 600

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
    if ($null -eq $Secrets) {
        return $Text
    }
    $values = @($Secrets.Values | Where-Object { -not [string]::IsNullOrEmpty($_) -and $_.Length -ge 4 } | Sort-Object { $_.Length } -Descending)
    foreach ($value in $values) {
        $Text = $Text.Replace([string]$value, "***")
    }
    return $Text
}

function Get-OtpSlot {
    $token = $env:CITEBOX_SIGN_SLOT
    if ([string]::IsNullOrWhiteSpace($token)) {
        $token = $env:RUNNER_NAME
    }
    if ([string]::IsNullOrWhiteSpace($token)) {
        $token = [Environment]::MachineName
    }
    if ($token -eq "desktop") {
        return 1
    }
    if ($token -eq "server") {
        return 0
    }
    $sum = 0
    foreach ($ch in $token.ToCharArray()) {
        $sum += [int]$ch
    }
    return $sum % 2
}

function Get-OtpRetryDelaySeconds {
    param(
        [int]$BaseSeconds,
        [int]$Slot
    )
    if ($BaseSeconds -le 0) {
        return 0
    }
    if ($Slot -lt 0) {
        $Slot = 0
    }
    # 35s leaves the current 30s TOTP window. Slot 1 waits one extra window so
    # parallel Windows desktop and server jobs retry with different codes.
    return $BaseSeconds + ($Slot * 30)
}

function Get-CodeSignDisposition {
    param(
        [int]$ExitCode,
        [string]$Output,
        [bool]$SignedFilePresent
    )
    $text = [string]$Output
    if ($text -match '(?i)invalid\s+otp' -or $text -match '(?i)otp\s+is\s+invalid') {
        return "retry-otp"
    }
    $hasError = $text -match '(?im)(^|\r?\n)\s*Error\s*:' -or $text -match '(?i)(\bException\b|java\.lang\.)'
    $hasSuccess = $text -match '(?i)Code signed successfully\s*:'
    if ($ExitCode -eq 0 -and $SignedFilePresent -and $hasSuccess -and -not $hasError) {
        return "success"
    }
    return "fail"
}

function Format-CapturedOutput {
    param(
        $Result,
        [string]$LogText
    )
    $parts = New-Object System.Collections.Generic.List[string]
    if ($Result.TimedOut) {
        $parts.Add("CodeSignTool timed out.")
    }
    if (-not [string]::IsNullOrWhiteSpace($Result.StdOut)) {
        $parts.Add("stdout:`n$($Result.StdOut.TrimEnd())")
    }
    if (-not [string]::IsNullOrWhiteSpace($Result.StdErr)) {
        $parts.Add("stderr:`n$($Result.StdErr.TrimEnd())")
    }
    if (-not [string]::IsNullOrWhiteSpace($LogText)) {
        $parts.Add("log:`n$($LogText.TrimEnd())")
    }
    if ($parts.Count -eq 0) {
        return "(no stdout, stderr, or log)"
    }
    return ($parts -join "`n")
}

function Invoke-CapturedProcess {
    param(
        [Parameter(Mandatory = $true)][string]$FileName,
        [string[]]$ArgumentList = @(),
        [string]$WorkingDirectory,
        [hashtable]$Environment,
        [int]$TimeoutSeconds = 600
    )
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $FileName
    $psi.UseShellExecute = $false
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.RedirectStandardInput = $true
    $psi.CreateNoWindow = $true
    $utf8 = New-Object System.Text.UTF8Encoding $false
    $psi.StandardOutputEncoding = $utf8
    $psi.StandardErrorEncoding = $utf8
    if ($WorkingDirectory) {
        $psi.WorkingDirectory = $WorkingDirectory
    }
    if ($null -eq $psi.ArgumentList) {
        throw "PowerShell 7 is required to sign Windows binaries (ProcessStartInfo.ArgumentList is missing)."
    }
    foreach ($arg in $ArgumentList) {
        [void]$psi.ArgumentList.Add([string]$arg)
    }
    if ($Environment) {
        foreach ($key in @($Environment.Keys)) {
            $psi.Environment[[string]$key] = [string]$Environment[$key]
        }
    }

    $process = New-Object System.Diagnostics.Process
    $process.StartInfo = $psi
    [void]$process.Start()
    $process.StandardInput.Close()
    $stdoutTask = $process.StandardOutput.ReadToEndAsync()
    $stderrTask = $process.StandardError.ReadToEndAsync()
    $timedOut = -not $process.WaitForExit($TimeoutSeconds * 1000)
    if ($timedOut) {
        try {
            $process.Kill($true)
        } catch {
            try { $process.Kill() } catch {}
        }
        $process.WaitForExit()
    } else {
        # WaitForExit(int) can return before async readers finish.
        $process.WaitForExit()
    }
    $stdout = $stdoutTask.GetAwaiter().GetResult()
    $stderr = $stderrTask.GetAwaiter().GetResult()
    $code = if ($timedOut) { 124 } else { $process.ExitCode }
    return @{
        ExitCode = $code
        StdOut = [string]$stdout
        StdErr = [string]$stderr
        TimedOut = [bool]$timedOut
    }
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
    param(
        [Parameter(Mandatory = $true)][string]$File,
        [hashtable]$Secrets
    )
    $signature = Get-AuthenticodeSignature -LiteralPath $File
    if ($null -eq $signature.SignerCertificate) {
        throw (Hide-SecretValues "$File has no Authenticode signer certificate." $Secrets)
    }
    if ($signature.Status -ne "Valid") {
        throw (Hide-SecretValues "$File Authenticode status is $($signature.Status): $($signature.StatusMessage)" $Secrets)
    }
    if ($null -eq $signature.TimeStamperCertificate) {
        throw (Hide-SecretValues "$File is signed but has no RFC3161 timestamp certificate." $Secrets)
    }

    $signTool = Find-SignTool
    if ($signTool) {
        $verify = Invoke-CapturedProcess -FileName $signTool -ArgumentList @("verify", "/pa", "/v", $File) -TimeoutSeconds 120
        $verifyText = Format-CapturedOutput $verify ""
        if ($verify.TimedOut -or $verify.ExitCode -ne 0) {
            throw (Hide-SecretValues "signtool verify failed for ${File} (exit $($verify.ExitCode)). $verifyText" $Secrets)
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
        throw "CodeSignTool $CodeSignToolVersion checksum mismatch."
    }
    Expand-Archive -LiteralPath $zipPath -DestinationPath $toolRoot -Force
    Remove-Item -LiteralPath $zipPath -Force

    if (-not (Test-Path -LiteralPath $jar) -or -not (Test-Path -LiteralPath $propertiesPath)) {
        throw "CodeSignTool archive did not contain the expected jar and conf/code_sign_tool.properties."
    }
    return @{ Root = $toolRoot; Jar = $jar; Properties = $propertiesPath }
}

function Read-CodeSignToolLog {
    param([Parameter(Mandatory = $true)][string]$ToolRoot)
    $logPath = Join-Path $ToolRoot "logs\code_signing_tool.log"
    if (-not (Test-Path -LiteralPath $logPath)) {
        return ""
    }
    try {
        $text = [IO.File]::ReadAllText($logPath)
    } catch {
        return ""
    }
    if ($text.Length -gt 16384) {
        $text = "(log truncated to last 16384 characters)`n" + $text.Substring($text.Length - 16384)
    }
    return $text
}

function Clear-CodeSignToolLog {
    param([Parameter(Mandatory = $true)][string]$ToolRoot)
    $logDir = Join-Path $ToolRoot "logs"
    if (Test-Path -LiteralPath $logDir) {
        Remove-Item -LiteralPath $logDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Find-SignedCopy {
    param(
        [Parameter(Mandatory = $true)][string]$OutputDir,
        [Parameter(Mandatory = $true)][string]$Leaf
    )
    $direct = Join-Path $OutputDir $Leaf
    if (Test-Path -LiteralPath $direct -PathType Leaf) {
        return $direct
    }
    $found = @(Get-ChildItem -LiteralPath $OutputDir -Recurse -File -ErrorAction SilentlyContinue | Where-Object { $_.Name -eq $Leaf })
    if ($found.Count -eq 1) {
        return $found[0].FullName
    }
    return ""
}

function Format-CodeSignFailure {
    param(
        [string]$Leaf,
        $Result,
        [string]$LogText,
        [bool]$SignedFilePresent
    )
    $reason = if ($Result.TimedOut) {
        "timed out"
    } elseif (-not $SignedFilePresent) {
        "did not write a signed copy"
    } else {
        "rejected the signed result"
    }
    $body = Format-CapturedOutput $Result $LogText
    return "CodeSignTool failed to sign $Leaf ($reason, exit $($Result.ExitCode)).`n$body"
}

function Invoke-CodeSignFiles {
    param(
        [Parameter(Mandatory = $true)][string[]]$Files,
        [Parameter(Mandatory = $true)][hashtable]$Secrets,
        [Parameter(Mandatory = $true)][string]$ToolRoot,
        [Parameter(Mandatory = $true)][string]$JarPath,
        [string]$JavaPath = "java",
        [int]$OtpRetrySeconds = 35,
        [string]$ToolVersion = $CodeSignToolVersion,
        # Tests substitute the Windows-only Authenticode check.
        [scriptblock]$VerifySignedFile
    )
    $ErrorActionPreference = "Stop"
    $toolEnv = @{ CODE_SIGN_TOOL_PATH = $ToolRoot }
    $help = Invoke-CapturedProcess -FileName $JavaPath -ArgumentList @("-jar", $JarPath, "sign", "--help") -WorkingDirectory $ToolRoot -Environment $toolEnv -TimeoutSeconds 120
    $helpText = ($help.StdOut + "`n" + $help.StdErr)
    if ($help.TimedOut -or $help.ExitCode -ne 0 -or $helpText -notmatch '(?i)Usage:\s*CodeSignTool sign') {
        throw (Hide-SecretValues "CodeSignTool sign --help failed (exit $($help.ExitCode)).`n$(Format-CapturedOutput $help '')" $Secrets)
    }
    $passPin = $helpText -match '(?m)^\s*-pin\b' -or $helpText -match '-pin='
    if ($passPin) {
        Write-Host "CodeSignTool accepts -pin. ESIGNER_PIN will be passed to sign."
    } else {
        Write-Host "CodeSignTool sign has no -pin flag. ESIGNER_PIN stays required and is not placed on the command line."
    }

    $otpSlot = Get-OtpSlot
    foreach ($item in $Files) {
        $leaf = [IO.Path]::GetFileName($item)
        Write-Host "Signing $leaf with SSL.com eSigner CodeSignTool ($ToolVersion)"
        $succeeded = $false
        foreach ($attempt in 1..4) {
            $outputDir = Join-Path ([IO.Path]::GetTempPath()) ("citebox-signed-" + [guid]::NewGuid().ToString("n"))
            New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
            try {
                Clear-CodeSignToolLog $ToolRoot
                $arguments = @(
                    "-jar", $JarPath,
                    "sign",
                    "-username=$($Secrets.ESIGNER_USERNAME)",
                    "-password=$($Secrets.ESIGNER_PASSWORD)",
                    "-credential_id=$($Secrets.ESIGNER_CREDENTIAL_ID)",
                    "-totp_secret=$($Secrets.ESIGNER_TOTP_SECRET)",
                    "-input_file_path=$item",
                    "-output_dir_path=$outputDir"
                )
                if ($passPin) {
                    $arguments += "-pin=$($Secrets.ESIGNER_PIN)"
                }

                $result = Invoke-CapturedProcess -FileName $JavaPath -ArgumentList $arguments -WorkingDirectory $ToolRoot -Environment $toolEnv -TimeoutSeconds $CodeSignTimeoutSeconds
                $logText = Read-CodeSignToolLog $ToolRoot
                $toolText = (($result.StdOut, $result.StdErr) -join "`n")
                $signedPath = Find-SignedCopy -OutputDir $outputDir -Leaf $leaf
                $signedPresent = -not [string]::IsNullOrWhiteSpace($signedPath)
                $disposition = Get-CodeSignDisposition -ExitCode $result.ExitCode -Output $toolText -SignedFilePresent $signedPresent
                if ($result.TimedOut) {
                    $disposition = "fail"
                }

                if ($disposition -eq "success") {
                    if ($null -ne $VerifySignedFile) {
                        & $VerifySignedFile $signedPath
                    } else {
                        Assert-AuthenticodeSignature -File $signedPath -Secrets $Secrets
                    }
                    Move-Item -LiteralPath $signedPath -Destination $item -Force
                    $succeeded = $true
                    break
                }

                $failure = Hide-SecretValues (Format-CodeSignFailure -Leaf $leaf -Result $result -LogText $logText -SignedFilePresent $signedPresent) $Secrets
                if ($disposition -eq "retry-otp" -and $attempt -lt 4) {
                    $delay = Get-OtpRetryDelaySeconds -BaseSeconds $OtpRetrySeconds -Slot $otpSlot
                    Write-Host "eSigner rejected the one-time password (attempt $attempt/4). Waiting ${delay}s for the next TOTP window."
                    Write-Host $failure
                    if ($delay -gt 0) {
                        Start-Sleep -Seconds $delay
                    }
                    continue
                }
                throw $failure
            } finally {
                Clear-CodeSignToolLog $ToolRoot
                if (Test-Path -LiteralPath $outputDir) {
                    Remove-Item -LiteralPath $outputDir -Recurse -Force -ErrorAction SilentlyContinue
                }
            }
        }
        if (-not $succeeded) {
            throw (Hide-SecretValues "CodeSignTool failed to sign $leaf." $Secrets)
        }
    }
}

function Start-CiteboxWindowsAuthenticodeSign {
    param([string[]]$Files)
    $ErrorActionPreference = "Stop"
    $ProgressPreference = "SilentlyContinue"

    if ($null -eq $Files -or $Files.Count -lt 1 -or $Files.Count -gt 100) {
        throw "Signable file path is required (1-100 files)."
    }
    foreach ($item in $Files) {
        if (-not (Test-Path -LiteralPath $item -PathType Leaf)) {
            throw "Signable file not found: $item"
        }
        $extension = [IO.Path]::GetExtension($item).ToLowerInvariant()
        if ($ArchiveExtensions -contains $extension) {
            throw "Refusing to Authenticode-sign archive '$item'. Sign the binaries, then package them."
        }
        if ($BinaryExtensions -notcontains $extension) {
            throw "Refusing to Authenticode-sign '$item'. Expected a Windows binary ($($BinaryExtensions -join ', '))."
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
            throw "Windows Authenticode signing needs $($SecretNames -join ', '). Missing: $($missing -join ', ')."
        }
        Write-Warning "eSigner secrets are not configured. Skipping Authenticode signing."
        return
    }

    if ($env:OS -ne "Windows_NT") {
        throw "Authenticode signing must run on Windows."
    }
    $java = Get-Command java -ErrorAction SilentlyContinue
    if (-not $java) {
        throw "Java is required to run SSL.com CodeSignTool."
    }

    $tool = Install-CodeSignTool
    $properties = Get-Content -Raw -LiteralPath $tool.Properties
    if ($properties -notmatch "(?m)^TSA_URL=$([regex]::Escape($Rfc3161TimestampUrl))\s*$") {
        throw "CodeSignTool TSA_URL must be the SSL.com RFC3161 endpoint $Rfc3161TimestampUrl."
    }

    Invoke-CodeSignFiles -Files $Files -Secrets $secrets -ToolRoot $tool.Root -JarPath $tool.Jar -JavaPath $java.Source
}

if ($MyInvocation.InvocationName -ne '.') {
    try {
        Start-CiteboxWindowsAuthenticodeSign -Files $Path
    } catch {
        $secrets = @{}
        foreach ($name in $SecretNames) {
            $secrets[$name] = Get-EsignerSecretValue $name
        }
        $message = Hide-SecretValues ([string]$_.Exception.Message) $secrets
        if ([string]::IsNullOrWhiteSpace($message)) {
            $message = Hide-SecretValues ([string]$_.ToString()) $secrets
        }
        [Console]::Error.WriteLine($message)
        exit 1
    }
    exit 0
}
