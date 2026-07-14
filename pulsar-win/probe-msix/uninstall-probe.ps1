[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$ReceiptPath,
    [Parameter(Mandatory = $true)][string]$PickerFixture,
    [Parameter(Mandatory = $true)][string]$CleanupReceiptPath,
    [switch]$EvidenceCopied
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot "hardware-evidence-contract.ps1")

if (-not $EvidenceCopied) {
    throw "refusing cleanup until -EvidenceCopied explicitly confirms that runtime evidence is preserved"
}
$ResolvedReceipt = (Resolve-Path -LiteralPath $ReceiptPath).Path
$Receipt = Get-Content -LiteralPath $ResolvedReceipt -Raw | ConvertFrom-Json
$ExpectedFamily = Get-ProbePackageFamilyName
$ExpectedAUMID = "$ExpectedFamily!$script:ProbeApplicationID"
$ExpectedRuntimeRelative = "Packages\$ExpectedFamily\LocalState\PulsarProbe"
$RuntimeRoot = Join-Path $env:LOCALAPPDATA $ExpectedRuntimeRelative
if ([string]::IsNullOrWhiteSpace($CleanupReceiptPath)) {
    throw "cleanup receipt path must be non-empty"
}
$CleanupReceiptFullPath = [IO.Path]::GetFullPath($CleanupReceiptPath)
if (Test-Path -LiteralPath $CleanupReceiptFullPath) {
    throw "refusing to overwrite an existing cleanup receipt"
}
$ResolvedPickerFixture = (Resolve-Path -LiteralPath $PickerFixture).Path
if ((Test-ProbePathWithinRoot -Path $ResolvedReceipt -Root $RuntimeRoot) -or
    (Test-ProbePathWithinRoot -Path $ResolvedPickerFixture -Root $RuntimeRoot) -or
    (Test-ProbePathWithinRoot -Path $CleanupReceiptFullPath -Root $RuntimeRoot)) {
    throw "receipt and picker evidence must live outside the runtime root removed by cleanup"
}
if ($CleanupReceiptFullPath -ieq $ResolvedReceipt -or
    $CleanupReceiptFullPath -ieq $ResolvedPickerFixture) {
    throw "cleanup receipt path must be distinct from preserved input evidence"
}

if ([int]$Receipt.schemaVersion -ne 2 -or
    [string]$Receipt.packageIdentity -cne $script:ProbePackageIdentity -or
    [string]$Receipt.publisher -cne $script:ProbePublisher -or
    [string]$Receipt.packageFamilyName -cne $ExpectedFamily -or
    [string]$Receipt.applicationUserModelId -cne $ExpectedAUMID -or
    [string]$Receipt.runtimeRootRelativeToLocalAppData -cne $ExpectedRuntimeRelative -or
    [string]$Receipt.processorArchitecture -cne "X64" -or
    [string]$Receipt.trustLevel -cne "appContainer" -or
    [string]$Receipt.runtimeBehavior -cne "packagedClassicApp" -or
    [string]$Receipt.packageSha256 -cnotmatch '^[0-9a-f]{64}$' -or
    [string]$Receipt.signatureStatusAfterTrust -cne "Valid" -or
    [string]$Receipt.signerSubject -cne $script:ProbePublisher -or
    [bool]$Receipt.privateSigningMaterialIncluded) {
    throw "install receipt differs from the frozen probe cleanup contract"
}
if ([bool]$Receipt.signerTrustAdded -and
    [string]$Receipt.signerIssuer -cne [string]$Receipt.signerSubject) {
    throw "run-added signer trust is not the frozen self-signed certificate"
}
$ReceiptCapabilities = @($Receipt.capabilities | Sort-Object)
$ExpectedCapabilities = @($script:ProbeExpectedCapabilities | Sort-Object)
if (@(Compare-Object $ExpectedCapabilities $ReceiptCapabilities).Count -ne 0) {
    throw "install receipt capabilities differ from the frozen probe set"
}
$Thumbprint = ([string]$Receipt.signerThumbprint -replace '\s', '').ToUpperInvariant()
if ($Thumbprint -cnotmatch '^[0-9A-F]{40}$') {
    throw "install receipt signer thumbprint is invalid"
}
if ([string]$Receipt.packageFullName -cnotlike "$($script:ProbePackageIdentity)_*") {
    throw "install receipt package full name is outside the frozen identity"
}

$StoppedProcesses = @(Get-Process -Name "pulsar-win-probe-amd64" -ErrorAction SilentlyContinue)
if ($StoppedProcesses.Count -ne 0) {
    $StoppedProcesses | Stop-Process -Force
    $Deadline = [DateTime]::UtcNow.AddSeconds(15)
    do {
        Start-Sleep -Milliseconds 100
        $RemainingProcesses = @(Get-Process -Name "pulsar-win-probe-amd64" -ErrorAction SilentlyContinue)
    } while ($RemainingProcesses.Count -ne 0 -and [DateTime]::UtcNow -lt $Deadline)
    if ($RemainingProcesses.Count -ne 0) {
        throw "probe process remained after bounded cleanup stop"
    }
}

$InstalledPackages = @(Get-AppxPackage -Name $script:ProbePackageIdentity -ErrorAction SilentlyContinue)
if ($InstalledPackages.Count -gt 1) {
    throw "cleanup found multiple packages in the frozen Pulsar family"
}
foreach ($Installed in $InstalledPackages) {
    if ([string]$Installed.PackageFamilyName -cne $ExpectedFamily -or
        [string]$Installed.PackageFullName -cne [string]$Receipt.packageFullName) {
        throw "installed package differs from the install receipt; refusing broad removal"
    }
    Remove-AppxPackage -Package $Installed.PackageFullName
}

$RemovedPartialCount = 0
if (Test-Path -LiteralPath $RuntimeRoot -PathType Container) {
    $RemovedPartialCount = @(
        Get-ChildItem -LiteralPath $RuntimeRoot -Recurse -File -Force |
            Where-Object { $_.Extension -ceq ".partial" -or $_.Name.EndsWith(".partial.reason", [StringComparison]::Ordinal) }
    ).Count
    Remove-Item -LiteralPath $RuntimeRoot -Recurse -Force
}

$TrustPath = "Cert:\LocalMachine\TrustedPeople\$Thumbprint"
$TrustWasAdded = [bool]$Receipt.signerTrustAdded
if ($TrustWasAdded) {
    if (-not (Test-Path -LiteralPath $TrustPath)) {
        throw "run-added signer trust is already missing; cleanup provenance is incomplete"
    }
    $TrustedCertificate = Get-Item -LiteralPath $TrustPath
    if ($TrustedCertificate.Subject -cne $script:ProbePublisher -or
        $TrustedCertificate.Issuer -cne $TrustedCertificate.Subject) {
        throw "refusing to remove a trust certificate outside the frozen self-signed Publisher"
    }
    $EnhancedKeyUsages = @($TrustedCertificate.EnhancedKeyUsageList | ForEach-Object { [string]$_.ObjectId })
    if ($EnhancedKeyUsages -notcontains "1.3.6.1.5.5.7.3.3") {
        throw "refusing to remove a trust certificate without Code Signing EKU"
    }
    Remove-Item -LiteralPath $TrustPath -Force
}

$PickerAccess = Test-ProbePickerFixtureExclusiveAccess -Path $ResolvedPickerFixture -Delete
$Hotkey = Test-ProbeHotkeyAvailability
if (-not $Hotkey.Registered) {
    throw "Ctrl+Shift+R remains unavailable after cleanup: Win32=$($Hotkey.Win32Error)"
}

$ProcessAbsent = @(Get-Process -Name "pulsar-win-probe-amd64" -ErrorAction SilentlyContinue).Count -eq 0
$PackageAbsent = @(Get-AppxPackage -Name $script:ProbePackageIdentity -ErrorAction SilentlyContinue).Count -eq 0
$RuntimeRootAbsent = -not (Test-Path -LiteralPath $RuntimeRoot)
$RunAddedTrustAbsent = if ($TrustWasAdded) { -not (Test-Path -LiteralPath $TrustPath) } else { $true }
$RemainingPartialCount = if ($RuntimeRootAbsent) {
    0
} else {
    @(
        Get-ChildItem -LiteralPath $RuntimeRoot -Recurse -File -Force |
            Where-Object { $_.Extension -ceq ".partial" -or $_.Name.EndsWith(".partial.reason", [StringComparison]::Ordinal) }
    ).Count
}
if (-not $ProcessAbsent -or -not $PackageAbsent -or -not $RuntimeRootAbsent -or
    -not $RunAddedTrustAbsent -or $RemainingPartialCount -ne 0) {
    throw "post-evidence cleanup postconditions did not converge"
}

$Cleanup = [ordered]@{
    schemaVersion = 1
    verificationBoundary = "post-evidence-cleanup-only; not hardware scenario acceptance"
    cleanedAtUTC = [DateTime]::UtcNow.ToString("o")
    packageSha256 = [string]$Receipt.packageSha256
    packageIdentity = [string]$Receipt.packageIdentity
    packageFamilyName = [string]$Receipt.packageFamilyName
    processesStopped = $StoppedProcesses.Count
    processAbsent = $ProcessAbsent
    packageAbsent = $PackageAbsent
    signerTrustWasAddedByRun = $TrustWasAdded
    signerTrustAbsent = $RunAddedTrustAbsent
    runtimeRootAbsent = $RuntimeRootAbsent
    removedPartialCount = $RemovedPartialCount
    remainingPartialCount = $RemainingPartialCount
    hotkeyAvailable = [bool]$Hotkey.Registered
    hotkeyWin32Error = [int]$Hotkey.Win32Error
    pickerFixtureExclusiveAccess = [bool]$PickerAccess.Accessible
    pickerFixtureDeleted = [bool]$PickerAccess.Deleted
    pickerFixtureSizeBytes = [int64]$PickerAccess.SizeBytes
}
Write-ProbeEvidenceJSON -Value $Cleanup -Path $CleanupReceiptFullPath
$Cleanup
