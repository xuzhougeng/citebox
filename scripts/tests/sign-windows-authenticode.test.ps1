$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$scriptPath = Join-Path $PSScriptRoot "..\sign-windows-authenticode.ps1"
. $scriptPath

$failures = New-Object System.Collections.Generic.List[string]

function Assert-True {
    param([bool]$Condition, [string]$Label)
    if (-not $Condition) {
        $script:failures.Add($Label)
        Write-Host "FAIL $Label"
    } else {
        Write-Host "ok   $Label"
    }
}

function Assert-Equal {
    param($Actual, $Expected, [string]$Label)
    if ($Actual -ne $Expected) {
        $script:failures.Add("$Label expected [$Expected] actual [$Actual]")
        Write-Host "FAIL $Label expected [$Expected] actual [$Actual]"
    } else {
        Write-Host "ok   $Label"
    }
}

function Assert-SecretHidden {
    param([string]$Text, [string[]]$Secrets, [string]$Label)
    foreach ($secret in $Secrets) {
        if (-not [string]::IsNullOrEmpty($secret) -and $Text.Contains($secret)) {
            $script:failures.Add("$Label leaked a secret")
            Write-Host "FAIL $Label leaked a secret"
            return
        }
    }
    Write-Host "ok   $Label"
}

$parseErrors = $null
$null = [System.Management.Automation.Language.Parser]::ParseFile((Resolve-Path $scriptPath), [ref]$null, [ref]$parseErrors)
Assert-True ($null -eq $parseErrors -or $parseErrors.Count -eq 0) "signer script parses"

Assert-Equal (Get-CodeSignDisposition -ExitCode 0 -Output "Error: The provided authorization grant is invalid, expired, revoked" -SignedFilePresent $false) "fail" "exit 0 plus Error is failure"
Assert-Equal (Get-CodeSignDisposition -ExitCode 0 -Output "Error: invalid otp" -SignedFilePresent $false) "retry-otp" "invalid otp retries when exit code is 0"
Assert-Equal (Get-CodeSignDisposition -ExitCode 1 -Output "Error: The OTP is invalid." -SignedFilePresent $false) "retry-otp" "OTP is invalid retries"
Assert-Equal (Get-CodeSignDisposition -ExitCode 0 -Output "Code signed successfully: C:\out\citebox.exe" -SignedFilePresent $true) "success" "success line and file is success"
Assert-Equal (Get-CodeSignDisposition -ExitCode 0 -Output "Code signed successfully: C:\out\citebox.exe" -SignedFilePresent $false) "fail" "success line without file is failure"
Assert-Equal (Get-CodeSignDisposition -ExitCode 0 -Output "java.lang.IllegalArgumentException: Illegal base64 character" -SignedFilePresent $false) "fail" "exception text is failure"
Assert-Equal (Get-CodeSignDisposition -ExitCode 3 -Output "Code signed successfully: C:\out\citebox.exe" -SignedFilePresent $true) "fail" "non-zero exit is failure"
Assert-Equal (Get-CodeSignDisposition -ExitCode 0 -Output "WARNING: sun.reflect.Reflection.getCallerClass is not supported. This will impact performance.`nCode signed successfully: C:\out\citebox.exe" -SignedFilePresent $true) "success" "JVM warning does not hide success"

$long = "Bearer longer-secret-value"
$short = "1234"
$redacted = Hide-SecretValues "pin 1234 and Bearer longer-secret-value" @{ ESIGNER_PIN = $short; ESIGNER_PASSWORD = $long }
Assert-True ((-not $redacted.Contains($long)) -and (-not $redacted.Contains($short)) -and $redacted.Contains("***")) "longer secrets are redacted before shorter ones"

Assert-Equal (Get-OtpRetryDelaySeconds -BaseSeconds 35 -Slot 0) 35 "server slot waits one TOTP window"
Assert-Equal (Get-OtpRetryDelaySeconds -BaseSeconds 35 -Slot 1) 65 "desktop slot waits an extra window"
Assert-Equal (Get-OtpRetryDelaySeconds -BaseSeconds 0 -Slot 1) 0 "tests can disable the OTP wait"
$previousSlot = $env:CITEBOX_SIGN_SLOT
$env:CITEBOX_SIGN_SLOT = "desktop"
Assert-Equal (Get-OtpSlot) 1 "desktop sign slot"
$env:CITEBOX_SIGN_SLOT = "server"
Assert-Equal (Get-OtpSlot) 0 "server sign slot"
if ($null -eq $previousSlot) {
    Remove-Item Env:CITEBOX_SIGN_SLOT -ErrorAction SilentlyContinue
} else {
    $env:CITEBOX_SIGN_SLOT = $previousSlot
}

$capture = Invoke-CapturedProcess -FileName "python3" -ArgumentList @(
    "-c",
    'import sys; sys.stdout.write("out-line\n"); sys.stderr.write("err-line\n"); sys.exit(7)'
) -TimeoutSeconds 30
Assert-Equal $capture.ExitCode 7 "captured process keeps the native exit code"
Assert-True ($capture.StdOut.Contains("out-line") -and $capture.StdErr.Contains("err-line")) "captured process keeps stdout and stderr"
$secretArg = '-password=P!@^^ss word"12\'
$quoted = Invoke-CapturedProcess -FileName "python3" -ArgumentList @(
    "-c",
    'import sys; sys.stdout.write("\n".join(sys.argv[1:]))',
    $secretArg
) -TimeoutSeconds 30
Assert-Equal $quoted.StdOut $secretArg "process arguments keep password punctuation"

$timeout = Invoke-CapturedProcess -FileName "python3" -ArgumentList @("-c", "import time; time.sleep(30)") -TimeoutSeconds 1
Assert-True ($timeout.TimedOut -and $timeout.ExitCode -eq 124) "process timeout is reported"

function New-FakeCodeSignTool {
    $root = Join-Path ([IO.Path]::GetTempPath()) ("citebox-cst-" + [guid]::NewGuid().ToString("n"))
    New-Item -ItemType Directory -Path (Join-Path $root "jar") -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $root "conf") -Force | Out-Null
    Set-Content -Path (Join-Path $root "conf\code_sign_tool.properties") -Value "TSA_URL=http://ts.ssl.com`n" -Encoding ascii
    Set-Content -Path (Join-Path $root "jar\code_sign_tool-1.3.2.jar") -Value "unused" -Encoding ascii
    $count = Join-Path $root "count.txt"
    Set-Content -Path $count -Value "0" -Encoding ascii
    $argv = Join-Path $root "argv.txt"
    $python = @'
#!/usr/bin/env python3
import os
import sys

root = os.environ["CODE_SIGN_TOOL_PATH"]
args = sys.argv[1:]
with open(os.path.join(root, "argv.txt"), "a", encoding="utf-8") as handle:
    handle.write("\n".join(args) + "\n---\n")
if "--help" in args:
    sys.stdout.write(os.environ.get("FAKE_JAVA_HELP", "Usage: CodeSignTool sign [-hV]\n      -password=<password>\n"))
    raise SystemExit(0)
mode = os.environ.get("FAKE_JAVA_MODE", "auth-error")
count_path = os.path.join(root, "count.txt")
count = int(open(count_path, encoding="utf-8").read().strip() or "0") + 1
open(count_path, "w", encoding="utf-8").write(str(count))
os.makedirs(os.path.join(root, "logs"), exist_ok=True)
values = {}
for arg in args:
    if arg.startswith("-") and "=" in arg:
        key, value = arg.split("=", 1)
        values[key] = value
cred = values.get("-credential_id", "")
password = values.get("-password", "")
with open(os.path.join(root, "logs", "code_signing_tool.log"), "w", encoding="utf-8") as handle:
    handle.write("Credential ID in input parameter: %s\n" % cred)
    handle.write("detail: signing attempt %s\n" % count)
if mode == "auth-error":
    sys.stdout.write("Error: authorization grant is invalid for %s\n" % password)
    raise SystemExit(0)
if mode == "invalid-otp":
    sys.stdout.write("Error: invalid otp\n")
    raise SystemExit(0)
if mode == "success":
    output = values["-output_dir_path"]
    leaf = os.path.basename(values["-input_file_path"])
    os.makedirs(output, exist_ok=True)
    with open(os.path.join(output, leaf), "w", encoding="utf-8") as handle:
        handle.write("signed-bytes")
    sys.stdout.write("Code signed successfully: %s\n" % os.path.join(output, leaf))
    raise SystemExit(0)
sys.stderr.write("java.lang.IllegalStateException: unexpected mode\n")
raise SystemExit(0)
'@
    $fake = Join-Path $root "fake-java"
    Set-Content -Path $fake -Value $python -Encoding ascii
    chmod +x $fake
    return @{ Root = $root; Jar = (Join-Path $root "jar\code_sign_tool-1.3.2.jar"); Fake = $fake; Count = $count; Argv = $argv }
}

$sampleSecrets = @{
    ESIGNER_USERNAME = "user@example.com"
    ESIGNER_PASSWORD = 'P!@^^ss word"12\'
    ESIGNER_CREDENTIAL_ID = "cred-id-9f3a"
    ESIGNER_TOTP_SECRET = "ii5gVvZ9G+WkxB3FauAnoL/z14AXSMistcE0jZMWWNSjQDlql2kt2D6Z+l8="
    ESIGNER_PIN = "9876"
}
$secretValues = @(
    $sampleSecrets.ESIGNER_USERNAME,
    $sampleSecrets.ESIGNER_PASSWORD,
    $sampleSecrets.ESIGNER_CREDENTIAL_ID,
    $sampleSecrets.ESIGNER_TOTP_SECRET,
    $sampleSecrets.ESIGNER_PIN
)

function Invoke-FakeSign {
    param(
        [hashtable]$Tool,
        [string]$Mode,
        [string]$File,
        [string]$Help = "Usage: CodeSignTool sign [-hV]`n      -password=<password>`n",
        [scriptblock]$Verify
    )
    $previousMode = $env:FAKE_JAVA_MODE
    $previousHelp = $env:FAKE_JAVA_HELP
    $env:FAKE_JAVA_MODE = $Mode
    $env:FAKE_JAVA_HELP = $Help
    try {
        Invoke-CodeSignFiles -Files @($File) -Secrets $sampleSecrets -ToolRoot $Tool.Root -JarPath $Tool.Jar -JavaPath $Tool.Fake -OtpRetrySeconds 0 -ToolVersion "v1.3.2" -VerifySignedFile $Verify
    } finally {
        if ($null -eq $previousMode) { Remove-Item Env:FAKE_JAVA_MODE -ErrorAction SilentlyContinue } else { $env:FAKE_JAVA_MODE = $previousMode }
        if ($null -eq $previousHelp) { Remove-Item Env:FAKE_JAVA_HELP -ErrorAction SilentlyContinue } else { $env:FAKE_JAVA_HELP = $previousHelp }
    }
}

$unsigned = Join-Path ([IO.Path]::GetTempPath()) ("citebox-unsigned-" + [guid]::NewGuid().ToString("n") + ".exe")
Set-Content -Path $unsigned -Value "unsigned" -Encoding ascii

$authTool = New-FakeCodeSignTool
$authMessage = ""
try {
    Invoke-FakeSign -Tool $authTool -Mode "auth-error" -File $unsigned
    $authMessage = "expected auth failure"
} catch {
    $authMessage = [string]$_.Exception.Message
}
Assert-True ($authMessage.Contains("exit 0") -and $authMessage.Contains("did not write a signed copy") -and $authMessage.Contains("Error: authorization grant is invalid") -and $authMessage.Contains("detail: signing attempt")) "exit 0 auth failure includes CodeSignTool output"
Assert-SecretHidden $authMessage $secretValues "auth failure output hides secrets"
$authArgs = Get-Content -Raw -LiteralPath $authTool.Argv
Assert-True ($authArgs.Contains("-password=$($sampleSecrets.ESIGNER_PASSWORD)")) "password is passed as one argument"
Assert-True (-not $authArgs.Contains("-pin=")) "pin is omitted when sign --help has no -pin flag"
Assert-Equal (Get-Content -Raw -LiteralPath $authTool.Count).Trim() "1" "auth failure does not retry"

$otpTool = New-FakeCodeSignTool
$otpMessage = ""
try {
    Invoke-FakeSign -Tool $otpTool -Mode "invalid-otp" -File $unsigned
    $otpMessage = "expected otp failure"
} catch {
    $otpMessage = [string]$_.Exception.Message
}
Assert-Equal (Get-Content -Raw -LiteralPath $otpTool.Count).Trim() "4" "invalid otp retries four times when the tool exits 0"
Assert-True ($otpMessage.Contains("Error: invalid otp")) "final otp failure includes the tool error"
Assert-SecretHidden $otpMessage $secretValues "otp failure output hides secrets"

$pinHelp = "Usage: CodeSignTool sign [-hV]`n      -pin=<pin>`n      -password=<password>`n"
$pinTool = New-FakeCodeSignTool
$pinMessage = ""
try {
    Invoke-FakeSign -Tool $pinTool -Mode "auth-error" -File $unsigned -Help $pinHelp
    $pinMessage = "expected pin failure"
} catch {
    $pinMessage = [string]$_.Exception.Message
}
$pinArgs = Get-Content -Raw -LiteralPath $pinTool.Argv
Assert-True ($pinArgs.Contains("-pin=$($sampleSecrets.ESIGNER_PIN)")) "pin is passed when sign --help offers it"
Assert-SecretHidden $pinMessage $secretValues "pin failure output hides the pin"

$script:verified = $false
$successTool = New-FakeCodeSignTool
Invoke-FakeSign -Tool $successTool -Mode "success" -File $unsigned -Verify {
    param($SignedPath)
    $script:verified = $true
    Assert-Equal (Get-Content -Raw -LiteralPath $SignedPath) "signed-bytes" "verifier sees the signed bytes"
}
Assert-True $script:verified "success path verifies before replacing the binary"
Assert-Equal (Get-Content -Raw -LiteralPath $unsigned) "signed-bytes" "signed bytes replace the unsigned file"

function Invoke-SignerProcess {
    param([string]$File, [hashtable]$EnvVars)
    $pwsh = (Get-Process -Id $PID).Path
    $previous = @{}
    foreach ($key in $EnvVars.Keys) {
        $previous[$key] = [Environment]::GetEnvironmentVariable($key)
        [Environment]::SetEnvironmentVariable($key, [string]$EnvVars[$key])
    }
    try {
        return Invoke-CapturedProcess -FileName $pwsh -ArgumentList @("-NoProfile", "-File", (Resolve-Path $scriptPath), "-Path", $File) -TimeoutSeconds 60
    } finally {
        foreach ($key in $EnvVars.Keys) {
            [Environment]::SetEnvironmentVariable($key, $previous[$key])
        }
    }
}

$cleared = @{}
foreach ($name in @("ESIGNER_CREDENTIAL_ID", "ESIGNER_USERNAME", "ESIGNER_PASSWORD", "ESIGNER_TOTP_SECRET", "ESIGNER_PIN", "CITEBOX_REQUIRE_WINDOWS_SIGNING")) {
    $cleared[$name] = $null
}
$skipped = Invoke-SignerProcess -File $unsigned -EnvVars $cleared
Assert-Equal $skipped.ExitCode 0 "missing secrets skip signing when it is not required"
Assert-True (($skipped.StdOut + $skipped.StdErr).Contains("Skipping Authenticode signing")) "skip path explains that signing was skipped"

$partial = $cleared.Clone()
$partial["ESIGNER_USERNAME"] = $sampleSecrets.ESIGNER_USERNAME
$partial["CITEBOX_REQUIRE_WINDOWS_SIGNING"] = "0"
$partialResult = Invoke-SignerProcess -File $unsigned -EnvVars $partial
Assert-Equal $partialResult.ExitCode 1 "partial secrets still fail the build"
Assert-True (($partialResult.StdErr).Contains("Missing:")) "partial secret failure lists the missing names"
Assert-SecretHidden ($partialResult.StdOut + $partialResult.StdErr) @($sampleSecrets.ESIGNER_USERNAME) "partial secret failure hides the provided secret"

$required = @{}
foreach ($key in $sampleSecrets.Keys) { $required[$key] = $sampleSecrets[$key] }
$required["CITEBOX_REQUIRE_WINDOWS_SIGNING"] = "1"
$windows = Invoke-SignerProcess -File $unsigned -EnvVars $required
Assert-Equal $windows.ExitCode 1 "tag signing refuses to run off Windows"
Assert-True ($windows.StdErr.Contains("Authenticode signing must run on Windows.")) "off-Windows failure names the platform requirement"
Assert-SecretHidden ($windows.StdOut + $windows.StdErr) $secretValues "off-Windows failure hides secrets"

Remove-Item -LiteralPath $unsigned -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $authTool.Root, $otpTool.Root, $pinTool.Root, $successTool.Root -Recurse -Force -ErrorAction SilentlyContinue

if ($failures.Count -gt 0) {
    Write-Host "$($failures.Count) assertion(s) failed."
    exit 1
}
Write-Host "sign-windows-authenticode tests passed"
