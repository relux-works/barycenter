[CmdletBinding()]
param([string]$Package = "")

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot "hardware-evidence-contract.ps1")

function Assert-True {
    param([Parameter(Mandatory = $true)][bool]$Condition, [Parameter(Mandatory = $true)][string]$Message)
    if (-not $Condition) { throw $Message }
}

function Assert-Throws {
    param(
        [Parameter(Mandatory = $true)][scriptblock]$Action,
        [Parameter(Mandatory = $true)][string]$MessagePattern
    )
    $Caught = $null
    try {
        & $Action
    } catch {
        $Caught = $_
    }
    if ($null -eq $Caught) {
        throw "expected failure matching '$MessagePattern'"
    }
    if ($Caught.Exception.Message -notmatch $MessagePattern) {
        throw "failure '$($Caught.Exception.Message)' does not match '$MessagePattern'"
    }
}

Assert-ProbeEvidenceRunID -RunID "win10-lab-a" | Out-Null
Assert-ProbeEvidenceScenario -Scenario "H17" | Out-Null
Assert-ProbeEvidenceFileName -Name "wack-report.xml" | Out-Null
Assert-Throws { Assert-ProbeEvidenceRunID -RunID "../escape" } "run ID"
Assert-Throws { Assert-ProbeEvidenceScenario -Scenario "H18" } "outside the frozen"
Assert-Throws { Assert-ProbeEvidenceFileName -Name "signer.pfx" } "forbidden"
Assert-Throws { Assert-ProbeEvidenceFileName -Name "nested/path.json" } "safe single"
Assert-ProbeEvidenceRelativeFile -RelativeFile "attachments/H00/install.json" | Out-Null
Assert-Throws { Assert-ProbeEvidenceRelativeFile -RelativeFile "attachments/../install.json" } "safe single"

$LTSC = Assert-ProbeHostIdentity `
    -OSFamily windows10 `
    -ProductName "Windows 10 Enterprise LTSC 2021" `
    -BuildNumber 19044 `
    -Architecture AMD64 `
    -Manufacturer "Physical Vendor" `
    -Model "Workstation 1" `
    -DeveloperModeValue 0 `
    -PhysicalMachineAttested $true `
    -ConsoleOperatorAttested $true
Assert-True ($LTSC -ceq "windows10-enterprise-ltsc-2021-mainstream") "LTSC posture mismatch"

Assert-Throws {
    Assert-ProbeHostIdentity `
        -OSFamily windows10 `
        -ProductName "Windows 10 Pro" `
        -BuildNumber 19045 `
        -Architecture AMD64 `
        -Manufacturer "Physical Vendor" `
        -Model "Workstation 2" `
        -DeveloperModeValue 0 `
        -PhysicalMachineAttested $true `
        -ConsoleOperatorAttested $true
} "strict Windows 10 posture"

$ExceptionPosture = Assert-ProbeHostIdentity `
    -OSFamily windows10 `
    -ProductName "Windows 10 Pro" `
    -BuildNumber 19045 `
    -Architecture AMD64 `
    -Manufacturer "Physical Vendor" `
    -Model "Workstation 2" `
    -DeveloperModeValue 0 `
    -PhysicalMachineAttested $true `
    -ConsoleOperatorAttested $true `
    -Windows10Posture ApprovedException `
    -SupportDecisionReference "TASK-260712-1vtwkl approved lifecycle decision"
Assert-True ($ExceptionPosture -ceq "windows10-explicit-product-exception") "approved exception posture mismatch"

Assert-Throws {
    Assert-ProbeHostIdentity `
        -OSFamily windows10 `
        -ProductName "Windows 10 Pro" `
        -BuildNumber 19045 `
        -Architecture AMD64 `
        -Manufacturer "Physical Vendor" `
        -Model "Workstation 2" `
        -DeveloperModeValue 0 `
        -PhysicalMachineAttested $true `
        -ConsoleOperatorAttested $true `
        -Windows10Posture ApprovedException
} "concise recorded product-decision"

Assert-Throws {
    Assert-ProbeHostIdentity `
        -OSFamily windows11 `
        -ProductName "Windows 11 Pro" `
        -BuildNumber 26100 `
        -Architecture AMD64 `
        -Manufacturer "Microsoft Corporation" `
        -Model "Virtual Machine" `
        -DeveloperModeValue 0 `
        -PhysicalMachineAttested $true `
        -ConsoleOperatorAttested $true
} "virtual or cloud"

Assert-Throws {
    Assert-ProbeHostIdentity `
        -OSFamily windows11 `
        -ProductName "Windows 11 Pro" `
        -BuildNumber 26100 `
        -Architecture AMD64 `
        -Manufacturer "Physical Vendor" `
        -Model "Workstation 3" `
        -DeveloperModeValue 1 `
        -PhysicalMachineAttested $true `
        -ConsoleOperatorAttested $true
} "Developer Mode"

Assert-Throws {
    Assert-ProbeHostIdentity `
        -OSFamily windows11 `
        -ProductName "Windows 11 Pro" `
        -BuildNumber 22631 `
        -Architecture AMD64 `
        -Manufacturer "Physical Vendor" `
        -Model "Workstation 3" `
        -DeveloperModeValue 0 `
        -PhysicalMachineAttested $true `
        -ConsoleOperatorAttested $true
} "24H2-or-later"

Assert-Throws {
    Assert-ProbeHostIdentity `
        -OSFamily windows11 `
        -ProductName "Windows 11 Pro" `
        -BuildNumber 26100 `
        -Architecture AMD64 `
        -Manufacturer "Physical Vendor" `
        -Model "Workstation 3" `
        -DeveloperModeValue 0 `
        -PhysicalMachineAttested $false `
        -ConsoleOperatorAttested $true
} "attestations are both required"

$Windows11 = Assert-ProbeHostIdentity `
    -OSFamily windows11 `
    -ProductName "Windows 11 Pro" `
    -BuildNumber 26100 `
    -Architecture AMD64 `
    -Manufacturer "Physical Vendor" `
    -Model "Workstation 3" `
    -DeveloperModeValue 0 `
    -PhysicalMachineAttested $true `
    -ConsoleOperatorAttested $true
Assert-True ($Windows11 -ceq "windows11-currently-serviced-operator-attested") "Windows 11 posture mismatch"

$TemporaryRoot = if ([string]::IsNullOrWhiteSpace($env:RUNNER_TEMP)) {
    [IO.Path]::GetTempPath()
} else {
    $env:RUNNER_TEMP
}
$TestRoot = Join-Path $TemporaryRoot "pulsar-hardware-evidence-$([guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Path $TestRoot | Out-Null
try {
    $ValidLog = Join-Path $TestRoot "scenarios.jsonl"
    $DeviceID = '\\?\SWD#MMDEVAPI#{0.0.1.00000000}.{1c403ff5-79ef-4c5b-a4fb-6fecc6c83a5a}'
    $ValidLines = @(
        ([ordered]@{
            timestamp = "2026-07-14T00:00:00Z"
            scenario = "capture"
            result = "pass"
            action = "capture_terminal"
            deviceId = $DeviceID
            fields = [ordered]@{ sessionId = "session-1"; path = "[redacted]"; sha256 = "abc123" }
        } | ConvertTo-Json -Compress -Depth 8),
        ([ordered]@{
            timestamp = "2026-07-14T00:00:01Z"
            scenario = "window"
            result = "attempt"
            action = "lifecycle_signal_observed"
            fields = [ordered]@{ cleanupId = 2; observedOSSignal = "WTS_SESSION_LOCK" }
        } | ConvertTo-Json -Compress -Depth 8)
    )
    [IO.File]::WriteAllLines($ValidLog, [string[]]$ValidLines, [Text.UTF8Encoding]::new($false))
    $LogContract = Assert-ProbeEvidenceJSONL -Path $ValidLog
    Assert-True ($LogContract.EventCount -eq 2) "valid JSONL event count mismatch"

    $PrivatePathLog = Join-Path $TestRoot "private-path.jsonl"
    $PrivatePathLine = [ordered]@{
        timestamp = "2026-07-14T00:00:00Z"
        scenario = "capture"
        result = "fail"
        action = "capture_terminal"
        failureCause = 'open C:\Users\alice\private.wav: denied'
    } | ConvertTo-Json -Compress
    [IO.File]::WriteAllText($PrivatePathLog, $PrivatePathLine, [Text.UTF8Encoding]::new($false))
    Assert-Throws { Assert-ProbeEvidenceJSONL -Path $PrivatePathLog } "absolute local path"

    $CredentialLog = Join-Path $TestRoot "credential.jsonl"
    $CredentialLine = [ordered]@{
        timestamp = "2026-07-14T00:00:00Z"
        scenario = "picker"
        result = "fail"
        action = "picker_result"
        failureCause = "authorization Bearer abc.def.ghi"
    } | ConvertTo-Json -Compress
    [IO.File]::WriteAllText($CredentialLog, $CredentialLine, [Text.UTF8Encoding]::new($false))
    Assert-Throws { Assert-ProbeEvidenceJSONL -Path $CredentialLog } "credential-like"

    Assert-Throws {
        Assert-ProbeEvidenceValueSafe -Value ([pscustomobject]@{ accessToken = "opaque-value" })
    } "not redacted"

    $BackwardLog = Join-Path $TestRoot "backward.jsonl"
    $BackwardLines = @(
        ([ordered]@{
            timestamp = "2026-07-14T00:00:02Z"
            scenario = "capture"
            result = "attempt"
            action = "capture_prepare"
        } | ConvertTo-Json -Compress),
        ([ordered]@{
            timestamp = "2026-07-14T00:00:01Z"
            scenario = "capture"
            result = "fail"
            action = "capture_terminal"
        } | ConvertTo-Json -Compress)
    )
    [IO.File]::WriteAllLines($BackwardLog, [string[]]$BackwardLines, [Text.UTF8Encoding]::new($false))
    Assert-Throws { Assert-ProbeEvidenceJSONL -Path $BackwardLog } "moves backward"

    $InvalidLog = Join-Path $TestRoot "invalid.jsonl"
    [IO.File]::WriteAllText($InvalidLog, "{invalid", [Text.UTF8Encoding]::new($false))
    Assert-Throws { Assert-ProbeEvidenceJSONL -Path $InvalidLog } "not valid JSON"

    $ManifestRoot = Join-Path $TestRoot "manifest"
    New-Item -ItemType Directory -Path $ManifestRoot | Out-Null
    [IO.File]::WriteAllText((Join-Path $ManifestRoot "safe.txt"), "safe", [Text.UTF8Encoding]::new($false))
    $Manifest = Get-ProbeEvidenceFileManifest -Root $ManifestRoot
    Assert-True ($Manifest.Count -eq 1 -and $Manifest[0].relativeFile -ceq "safe.txt") "safe manifest mismatch"
    [IO.File]::WriteAllText((Join-Path $ManifestRoot "signer.cer"), "public cert export", [Text.UTF8Encoding]::new($false))
    Assert-Throws { Get-ProbeEvidenceFileManifest -Root $ManifestRoot } "forbidden"

    $PickerFixture = Join-Path $TestRoot "picker-fixture.bin"
    [IO.File]::WriteAllBytes($PickerFixture, [byte[]](1, 2, 3, 4))
    $PickerResult = Test-ProbePickerFixtureExclusiveAccess -Path $PickerFixture
    Assert-True ($PickerResult.Accessible -and -not $PickerResult.Deleted -and $PickerResult.SizeBytes -eq 4) "picker fixture exclusive access mismatch"
    $DeletablePickerFixture = Join-Path $TestRoot "picker-fixture-delete.bin"
    [IO.File]::WriteAllBytes($DeletablePickerFixture, [byte[]](5, 6, 7, 8))
    $PickerDeleteResult = Test-ProbePickerFixtureExclusiveAccess -Path $DeletablePickerFixture -Delete
    Assert-True ($PickerDeleteResult.Accessible -and $PickerDeleteResult.Deleted) "picker fixture deletion mismatch"
    Assert-True (-not (Test-Path -LiteralPath $DeletablePickerFixture)) "deleted picker fixture remained"

    $HotkeyBefore = Test-ProbeHotkeyAvailability
    Assert-True ($HotkeyBefore.Registered -and $HotkeyBefore.Win32Error -eq 0) "Ctrl+Shift+R is unexpectedly unavailable before conflict test"

    $Ready = Join-Path $TestRoot "hotkey-ready.json"
    $PowerShellPath = (Get-Process -Id $PID).Path
    $Holder = Start-Process -FilePath $PowerShellPath -ArgumentList @(
        "-NoLogo", "-NoProfile", "-NonInteractive", "-File", (Join-Path $PSScriptRoot "hotkey-conflict.ps1"),
        "-Mode", "Hold", "-HoldSeconds", "30", "-ReadyPath", $Ready
    ) -PassThru
    try {
        $ReadyDeadline = [DateTime]::UtcNow.AddSeconds(15)
        while (-not (Test-Path -LiteralPath $Ready) -and -not $Holder.HasExited -and [DateTime]::UtcNow -lt $ReadyDeadline) {
            Start-Sleep -Milliseconds 100
            $Holder.Refresh()
        }
        if (-not (Test-Path -LiteralPath $Ready)) {
            throw "hotkey conflict holder did not become ready"
        }
        $Conflict = Test-ProbeHotkeyAvailability
        Assert-True (-not $Conflict.Registered) "second Ctrl+Shift+R registration unexpectedly succeeded"
        Assert-True ($Conflict.Win32Error -eq 1409) "hotkey conflict Win32=$($Conflict.Win32Error), expected 1409"
    } finally {
        if (-not $Holder.HasExited) { Stop-Process -Id $Holder.Id -Force }
        $Holder.WaitForExit()
        $Holder.Dispose()
    }
    $ReleaseDeadline = [DateTime]::UtcNow.AddSeconds(5)
    do {
        $HotkeyAfter = Test-ProbeHotkeyAvailability
        if (-not $HotkeyAfter.Registered) { Start-Sleep -Milliseconds 100 }
    } while (-not $HotkeyAfter.Registered -and [DateTime]::UtcNow -lt $ReleaseDeadline)
    Assert-True ($HotkeyAfter.Registered) "Ctrl+Shift+R remained unavailable after holder exit"

    $SyntheticRoot = Join-Path $TestRoot "synthetic-run"
    New-Item -ItemType Directory -Path $SyntheticRoot | Out-Null
    $SyntheticHash = [string]::new([char]'a', 64)
    Write-ProbeEvidenceJSON -Value ([ordered]@{
        schemaVersion = 1
        verificationBoundary = "contract-test-only"
        runId = "contract-test"
        osFamily = "windows11"
        packageSha256 = $SyntheticHash
    }) -Path (Join-Path $SyntheticRoot "run.json")
    Write-ProbeEvidenceJSON -Value ([ordered]@{
        schemaVersion = 1
        verificationBoundary = "contract-test-only"
        osFamily = "windows11"
    }) -Path (Join-Path $SyntheticRoot "machine.json")
    Write-ProbeEvidenceJSON -Value ([ordered]@{
        schemaVersion = 1
        verificationBoundary = "contract-test-only"
        sha256 = $SyntheticHash
    }) -Path (Join-Path $SyntheticRoot "package.json")
    $SyntheticRows = foreach ($ScenarioID in $script:ProbeEvidenceScenarios) {
        [ordered]@{
            id = $ScenarioID
            title = "contract-test"
            collectionState = "not-collected"
            verdict = "NOT_RUN"
            observation = $null
            nextAction = $null
            reviewState = "unreviewed"
            recordedAtUTC = $null
            evidence = @()
        }
    }
    Write-ProbeEvidenceJSON -Value ([ordered]@{
        schemaVersion = 1
        verificationBoundary = "contract-test-only"
        runId = "contract-test"
        osFamily = "windows11"
        packageSha256 = $SyntheticHash
        scenarios = @($SyntheticRows)
    }) -Path (Join-Path $SyntheticRoot "matrix.json")
    $HardwareEvidenceScript = Join-Path $PSScriptRoot "hardware-evidence.ps1"
    $OutOfOrderEvidence = Join-Path $TestRoot "out-of-order.txt"
    [IO.File]::WriteAllText($OutOfOrderEvidence, "out of order", [Text.UTF8Encoding]::new($false))
    Assert-Throws {
        & $HardwareEvidenceScript `
            -Mode Attach `
            -OutputDirectory $SyntheticRoot `
            -Scenario H01 `
            -Attachment $OutOfOrderEvidence
    } "strict current evidence scenario is H00"
    foreach ($ScenarioID in $script:ProbeEvidenceScenarios) {
        if ($ScenarioID -ceq "H17") {
            $SourceEvidence = Join-Path $TestRoot "cleanup.json"
            Write-ProbeEvidenceJSON -Value ([ordered]@{
                schemaVersion = 1
                verificationBoundary = "post-evidence-cleanup-only; not hardware scenario acceptance"
                packageSha256 = $SyntheticHash
                processAbsent = $true
                packageAbsent = $true
                signerTrustAbsent = $true
                runtimeRootAbsent = $true
                hotkeyAvailable = $true
                pickerFixtureExclusiveAccess = $true
                pickerFixtureDeleted = $true
                remainingPartialCount = 0
            }) -Path $SourceEvidence
        } else {
            $SourceEvidence = Join-Path $TestRoot "evidence-$ScenarioID.txt"
            [IO.File]::WriteAllText($SourceEvidence, "synthetic contract test $ScenarioID", [Text.UTF8Encoding]::new($false))
        }
        & $HardwareEvidenceScript `
            -Mode Attach `
            -OutputDirectory $SyntheticRoot `
            -Scenario $ScenarioID `
            -Attachment $SourceEvidence | Out-Null
        & $HardwareEvidenceScript `
            -Mode Verdict `
            -OutputDirectory $SyntheticRoot `
            -Scenario $ScenarioID `
            -Verdict PASS `
            -Observation "synthetic contract test only" | Out-Null
    }
    Assert-Throws {
        & $HardwareEvidenceScript `
            -Mode Verdict `
            -OutputDirectory $SyntheticRoot `
            -Scenario H00 `
            -Verdict PASS `
            -Observation "attempted overwrite"
    } "already has a terminal"
    $H00Attachment = Join-Path $SyntheticRoot "attachments\H00\evidence-H00.txt"
    $H00Original = [IO.File]::ReadAllText($H00Attachment)
    [IO.File]::WriteAllText($H00Attachment, "tampered", [Text.UTF8Encoding]::new($false))
    Assert-Throws {
        & $HardwareEvidenceScript -Mode Seal -OutputDirectory $SyntheticRoot
    } "referenced evidence hash changed"
    [IO.File]::WriteAllText($H00Attachment, $H00Original, [Text.UTF8Encoding]::new($false))
    $Seal = & $HardwareEvidenceScript -Mode Seal -OutputDirectory $SyntheticRoot
    Assert-True ($Seal.overallOperatorVerdict -ceq "all-operator-pass-unreviewed") "synthetic seal verdict mismatch"
    Assert-True ((Test-Path -LiteralPath (Join-Path $SyntheticRoot "bundle-manifest.json"))) "bundle manifest missing"
    Assert-True ((Test-Path -LiteralPath (Join-Path $SyntheticRoot "sealed.json"))) "seal receipt missing"

    if (-not [string]::IsNullOrWhiteSpace($Package)) {
        $PackageEvidence = Get-ProbePackageEvidence -Package $Package
        Assert-True ($PackageEvidence.packageIdentity -ceq "ReluxWorksLLC.PulsarBarycenter") "package evidence identity mismatch"
        Assert-True ($PackageEvidence.processorArchitecture -ceq "x64") "package evidence architecture mismatch"
        Assert-True ($PackageEvidence.sha256 -cmatch '^[0-9a-f]{64}$') "package evidence SHA-256 mismatch"
        Assert-True ($PackageEvidence.privateSigningMaterialIncluded -eq $false) "package evidence claimed private signing material"
    }
} finally {
    Remove-Item -LiteralPath $TestRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "Hardware evidence contract, privacy, lifecycle posture, package provenance, picker, and hotkey regressions passed."
exit 0
