$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

. (Join-Path $PSScriptRoot "package-contract.ps1")

$script:ProbeEvidenceScenarios = @(
    "H00", "H01", "H02", "H03", "H04", "H05", "H06", "H07", "H08",
    "H09", "H10", "H11", "H12", "H13", "H14", "H15", "H16", "H17"
)
$script:ProbeForbiddenEvidenceExtensions = @(
    ".cer", ".crt", ".der", ".key", ".p12", ".pem", ".pfx", ".pvk"
)
$script:ProbeWindowsPathPattern = [regex]::new(
    '(?i)(^|[\s("''=:\[])(?:[a-z]:[\\/]|\\\\[^\\\s]+[\\/])'
)
$script:ProbePosixPathPattern = [regex]::new('(^|[\s("''=\[])/')
$script:ProbeFileUriPattern = [regex]::new('(?i)\bfile:/+')
$script:ProbeCredentialPattern = [regex]::new(
    '(?i)(bearer\s+|(?:access[_-]?)?token(?:\s*[=:]\s*|\s+)|secret(?:\s*[=:]\s*|\s+)|api[_-]?key(?:\s*[=:]\s*|\s+)|password(?:\s*[=:]\s*|\s+)|passwd(?:\s*[=:]\s*|\s+)|credential(?:\s*[=:]\s*|\s+)|authorization(?:\s*[=:]\s*|\s+))[^\s,;]+'
)

if (-not ("Pulsar.ProbeHotkeyRegistration" -as [type])) {
    Add-Type -TypeDefinition @"
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;

namespace Pulsar
{
    public sealed class ProbeHotkeyRegistration : IDisposable
    {
        private const int HotkeyId = 0x5054;
        private const uint ModControl = 0x0002;
        private const uint ModShift = 0x0004;
        private const uint ModNoRepeat = 0x4000;
        private const uint VirtualKeyR = 0x52;

        [DllImport("user32.dll", SetLastError = true)]
        private static extern bool RegisterHotKey(IntPtr window, int id, uint modifiers, uint virtualKey);

        [DllImport("user32.dll", SetLastError = true)]
        private static extern bool UnregisterHotKey(IntPtr window, int id);

        public bool Registered { get; private set; }
        public int Win32Error { get; private set; }

        public ProbeHotkeyRegistration()
        {
            Registered = RegisterHotKey(IntPtr.Zero, HotkeyId, ModControl | ModShift | ModNoRepeat, VirtualKeyR);
            Win32Error = Registered ? 0 : Marshal.GetLastWin32Error();
        }

        public void Dispose()
        {
            if (Registered)
            {
                if (!UnregisterHotKey(IntPtr.Zero, HotkeyId))
                {
                    throw new Win32Exception(Marshal.GetLastWin32Error(), "UnregisterHotKey failed");
                }
                Registered = false;
            }
        }
    }
}
"@
}

function Assert-ProbeEvidenceRunID {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$RunID)

    if ($RunID -cnotmatch '^[a-z0-9][a-z0-9-]{0,47}$') {
        throw "evidence run ID must be 1-48 lowercase alphanumeric/hyphen characters"
    }
    $RunID
}

function Assert-ProbeEvidenceScenario {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Scenario)

    if ($script:ProbeEvidenceScenarios -cnotcontains $Scenario) {
        throw "evidence scenario '$Scenario' is outside the frozen H00-H17 matrix"
    }
    $Scenario
}

function Assert-ProbeEvidenceFileName {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Name)

    if ($Name -cnotmatch '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$' -or $Name.Contains("..")) {
        throw "evidence file name '$Name' is not a safe single path component"
    }
    if ($script:ProbeForbiddenEvidenceExtensions -ccontains [IO.Path]::GetExtension($Name).ToLowerInvariant()) {
        throw "certificate and key export '$Name' is forbidden in the evidence bundle"
    }
    $Name
}

function Assert-ProbeEvidenceRelativeFile {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$RelativeFile)

    if ([string]::IsNullOrWhiteSpace($RelativeFile) -or
        $RelativeFile.Contains('\') -or
        [IO.Path]::IsPathRooted($RelativeFile)) {
        throw "evidence relative file '$RelativeFile' must use safe forward-slash components"
    }
    $Components = @($RelativeFile.Split('/'))
    if ($Components.Count -eq 0 -or $Components.Count -gt 16) {
        throw "evidence relative file '$RelativeFile' has an invalid component count"
    }
    foreach ($Component in $Components) {
        Assert-ProbeEvidenceFileName -Name $Component | Out-Null
    }
    $Components -join '/'
}

function Write-ProbeEvidenceJSON {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)]$Value,
        [Parameter(Mandatory = $true)][string]$Path,
        [switch]$Replace
    )

    if ((Test-Path -LiteralPath $Path) -and -not $Replace) {
        throw "refusing to overwrite evidence file '$([IO.Path]::GetFileName($Path))'"
    }
    $Parent = Split-Path -Parent $Path
    if (-not [string]::IsNullOrWhiteSpace($Parent)) {
        New-Item -ItemType Directory -Force -Path $Parent | Out-Null
    }
    $JSON = $Value | ConvertTo-Json -Depth 20
    [IO.File]::WriteAllText(
        $Path,
        $JSON + [Environment]::NewLine,
        [Text.UTF8Encoding]::new($false)
    )
}

function Write-ProbeEvidenceText {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Value,
        [Parameter(Mandatory = $true)][string]$Path
    )

    if (Test-Path -LiteralPath $Path) {
        throw "refusing to overwrite evidence file '$([IO.Path]::GetFileName($Path))'"
    }
    $Parent = Split-Path -Parent $Path
    if (-not [string]::IsNullOrWhiteSpace($Parent)) {
        New-Item -ItemType Directory -Force -Path $Parent | Out-Null
    }
    [IO.File]::WriteAllText($Path, $Value, [Text.UTF8Encoding]::new($false))
}

function Test-ProbePathWithinRoot {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Root
    )

    $FullPath = [IO.Path]::GetFullPath($Path).TrimEnd('\', '/')
    $FullRoot = [IO.Path]::GetFullPath($Root).TrimEnd('\', '/')
    $Comparison = if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
        [StringComparison]::OrdinalIgnoreCase
    } else {
        [StringComparison]::Ordinal
    }
    if ($FullPath.Equals($FullRoot, $Comparison)) { return $true }
    $Prefix = $FullRoot + [IO.Path]::DirectorySeparatorChar
    $FullPath.StartsWith($Prefix, $Comparison)
}

function Test-ProbeSensitiveEvidenceKey {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Name)

    $Normalized = ($Name -replace '[_-]', '').ToLowerInvariant()
    foreach ($Fragment in @(
        "path", "filename", "originalname", "username", "userprofile", "token",
        "secret", "auth", "credential", "apikey", "password", "passwd", "cookie",
        "audiocontent", "recordingcontent", "pcmpayload", "samples"
    )) {
        if ($Normalized.Contains($Fragment)) { return $true }
    }
    $false
}

function Assert-ProbeEvidenceValueSafe {
    [CmdletBinding()]
    param(
        [AllowNull()]$Value,
        [string]$PropertyName = "value",
        [int]$Depth = 0
    )

    if ($Depth -gt 24) {
        throw "evidence value nesting exceeds the 24-level inspection bound"
    }
    if ($null -eq $Value) { return }

    if ($Value -is [string]) {
        if ((Test-ProbeSensitiveEvidenceKey -Name $PropertyName) -and
            $Value -cne "[redacted]") {
            throw "sensitive evidence field '$PropertyName' is not redacted"
        }
        $RecognizedDeviceID = $PropertyName -ceq "deviceId" -and (
            $Value.StartsWith('\\?\SWD#MMDEVAPI#', [StringComparison]::OrdinalIgnoreCase) -or
            $Value -cmatch '^\{[0-9]+\.[0-9]+\.[0-9]+\.[0-9a-fA-F]+\}\.\{[0-9a-fA-F-]+\}$'
        )
        if (-not $RecognizedDeviceID -and (
            $script:ProbeWindowsPathPattern.IsMatch($Value) -or
            $script:ProbePosixPathPattern.IsMatch($Value) -or
            $script:ProbeFileUriPattern.IsMatch($Value)
        )) {
            throw "evidence field '$PropertyName' contains an absolute local path"
        }
        if ($script:ProbeCredentialPattern.IsMatch($Value)) {
            throw "evidence field '$PropertyName' contains credential-like text"
        }
        return
    }

    if ($Value -is [System.Collections.IDictionary]) {
        foreach ($Key in $Value.Keys) {
            Assert-ProbeEvidenceValueSafe -Value $Value[$Key] -PropertyName ([string]$Key) -Depth ($Depth + 1)
        }
        return
    }

    if ($Value -is [System.Collections.IEnumerable]) {
        foreach ($Item in $Value) {
            Assert-ProbeEvidenceValueSafe -Value $Item -PropertyName $PropertyName -Depth ($Depth + 1)
        }
        return
    }

    if ($Value -is [pscustomobject]) {
        foreach ($Property in $Value.PSObject.Properties) {
            if ($Property.MemberType -in @("NoteProperty", "Property", "AliasProperty")) {
                Assert-ProbeEvidenceValueSafe -Value $Property.Value -PropertyName $Property.Name -Depth ($Depth + 1)
            }
        }
    }
}

function Assert-ProbeEvidenceJSONL {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "scenario JSONL is missing"
    }
    $Lines = @(Get-Content -LiteralPath $Path | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    if ($Lines.Count -eq 0) {
        throw "scenario JSONL is empty"
    }
    $FirstTimestamp = $null
    $LastTimestamp = $null
    $PreviousTimestamp = $null
    for ($Index = 0; $Index -lt $Lines.Count; $Index++) {
        try {
            $Event = $Lines[$Index] | ConvertFrom-Json
        } catch {
            throw "scenario JSONL line $($Index + 1) is not valid JSON: $($_.Exception.Message)"
        }
        foreach ($Required in @("timestamp", "scenario", "result", "action")) {
            if ($null -eq $Event.PSObject.Properties[$Required] -or
                [string]::IsNullOrWhiteSpace([string]$Event.$Required)) {
                throw "scenario JSONL line $($Index + 1) lacks '$Required'"
            }
        }
        $ParsedTimestamp = [DateTimeOffset]::MinValue
        if (-not [DateTimeOffset]::TryParse([string]$Event.timestamp, [ref]$ParsedTimestamp)) {
            throw "scenario JSONL line $($Index + 1) has an invalid timestamp"
        }
        if ($null -ne $PreviousTimestamp -and $ParsedTimestamp -lt $PreviousTimestamp) {
            throw "scenario JSONL line $($Index + 1) moves backward in time"
        }
        if ([string]$Event.scenario -cnotin @("permission", "capture", "hotkey", "picker", "window")) {
            throw "scenario JSONL line $($Index + 1) has an unknown scenario"
        }
        if ([string]$Event.result -cnotin @("attempt", "pass", "fail", "blocked", "discard")) {
            throw "scenario JSONL line $($Index + 1) has an unknown result"
        }
        Assert-ProbeEvidenceValueSafe -Value $Event
        if ($null -eq $FirstTimestamp) { $FirstTimestamp = $ParsedTimestamp }
        $LastTimestamp = $ParsedTimestamp
        $PreviousTimestamp = $ParsedTimestamp
    }
    [pscustomobject]@{
        EventCount = $Lines.Count
        FirstTimestampUTC = $FirstTimestamp.ToUniversalTime().ToString("o")
        LastTimestampUTC = $LastTimestamp.ToUniversalTime().ToString("o")
    }
}

function Get-ProbeEvidenceFileManifest {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Root,
        [string[]]$ExcludeRelativePath = @()
    )

    $ResolvedRoot = (Resolve-Path -LiteralPath $Root).Path.TrimEnd('\', '/')
    $Prefix = $ResolvedRoot + [IO.Path]::DirectorySeparatorChar
    $Items = @(Get-ChildItem -LiteralPath $ResolvedRoot -Recurse -Force)
    foreach ($Item in $Items) {
        if (($Item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "evidence bundles may not contain reparse points"
        }
    }
    $Manifest = @()
    foreach ($File in @($Items | Where-Object { -not $_.PSIsContainer } | Sort-Object FullName)) {
        $Relative = $File.FullName.Substring($Prefix.Length).Replace('\', '/')
        if ($ExcludeRelativePath -ccontains $Relative) { continue }
        Assert-ProbeEvidenceRelativeFile -RelativeFile $Relative | Out-Null
        $Manifest += [pscustomobject]@{
            relativeFile = $Relative
            sizeBytes = [int64]$File.Length
            sha256 = (Get-FileHash -LiteralPath $File.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
    @($Manifest)
}

function Get-ProbePackageEvidence {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Package)

    $PackagePath = (Resolve-Path -LiteralPath $Package).Path
    $Contract = Get-ProbePackageManifestContract -PackagePath $PackagePath
    $Signature = Get-AuthenticodeSignature -FilePath $PackagePath
    if ($null -eq $Signature.SignerCertificate) {
        throw "hardware evidence input package has no embedded signer certificate"
    }
    $Signer = $Signature.SignerCertificate
    if ([string]$Signature.Status -cnotin @("Valid", "NotTrusted", "UnknownError")) {
        throw "hardware evidence input signature status '$($Signature.Status)' is not admissible before controlled trust"
    }
    $EnhancedKeyUsages = @($Signer.EnhancedKeyUsageList | ForEach-Object { [string]$_.ObjectId })
    if ($EnhancedKeyUsages -notcontains "1.3.6.1.5.5.7.3.3") {
        throw "hardware evidence input signer lacks the Code Signing enhanced key usage"
    }
    $Now = Get-Date
    if ($Signer.NotBefore -gt $Now -or $Signer.NotAfter -le $Now) {
        throw "hardware evidence input signer is outside its validity window"
    }
    $SelfSignedLocal = $Signer.Subject -ceq $script:ProbePublisher -and $Signer.Issuer -ceq $Signer.Subject
    [ordered]@{
        schemaVersion = 1
        verificationBoundary = "signed-package-provenance-only; not physical hardware evidence"
        packageFile = [IO.Path]::GetFileName($PackagePath)
        sha256 = (Get-FileHash -LiteralPath $PackagePath -Algorithm SHA256).Hash.ToLowerInvariant()
        packageIdentity = $Contract.PackageIdentity
        publisher = $Contract.Publisher
        version = $Contract.Version
        processorArchitecture = $Contract.ProcessorArchitecture
        applicationId = $Contract.ApplicationID
        packageFamilyName = $Contract.PackageFamilyName
        applicationUserModelId = "$($Contract.PackageFamilyName)!$($Contract.ApplicationID)"
        trustLevel = $Contract.TrustLevel
        runtimeBehavior = $Contract.RuntimeBehavior
        capabilities = @($Contract.Capabilities)
        signatureStatusBeforeTrust = [string]$Signature.Status
        signerRoute = if ($SelfSignedLocal) { "self-signed-controlled-hardware" } else { "store-or-external-trust" }
        signerSubject = [string]$Signer.Subject
        signerIssuer = [string]$Signer.Issuer
        signerThumbprint = ([string]$Signer.Thumbprint).ToLowerInvariant()
        signerNotBeforeUTC = $Signer.NotBefore.ToUniversalTime().ToString("o")
        signerNotAfterUTC = $Signer.NotAfter.ToUniversalTime().ToString("o")
        signerCodeSigningEKU = $true
        privateSigningMaterialIncluded = $false
    }
}

function Assert-ProbeHostIdentity {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][ValidateSet("windows10", "windows11")][string]$OSFamily,
        [Parameter(Mandatory = $true)][string]$ProductName,
        [Parameter(Mandatory = $true)][int]$BuildNumber,
        [Parameter(Mandatory = $true)][string]$Architecture,
        [Parameter(Mandatory = $true)][string]$Manufacturer,
        [Parameter(Mandatory = $true)][string]$Model,
        [Parameter(Mandatory = $true)][int]$DeveloperModeValue,
        [Parameter(Mandatory = $true)][bool]$PhysicalMachineAttested,
        [Parameter(Mandatory = $true)][bool]$ConsoleOperatorAttested,
        [ValidateSet("EnterpriseLTSC2021", "ApprovedException")][string]$Windows10Posture = "EnterpriseLTSC2021",
        [string]$SupportDecisionReference = ""
    )

    if (-not $PhysicalMachineAttested -or -not $ConsoleOperatorAttested) {
        throw "physical-machine and physical-console operator attestations are both required"
    }
    if ($Architecture -cne "AMD64") {
        throw "hardware evidence requires the package's x64/AMD64 architecture, got '$Architecture'"
    }
    if ($DeveloperModeValue -ne 0) {
        throw "Developer Mode must be off for the hardware matrix"
    }
    $MachineDescription = "$Manufacturer $Model"
    if ($MachineDescription -match '(?i)\b(vmware|virtualbox|virtual machine|kvm|qemu|parallels|xen|bochs|cloud pc|ec2|compute engine)\b') {
        throw "machine manufacturer/model identifies a virtual or cloud host"
    }

    if ($OSFamily -ceq "windows10") {
        if ($BuildNumber -lt 19041 -or $BuildNumber -ge 22000) {
            throw "Windows 10 row requires build 19041-21999, got $BuildNumber"
        }
        if ($Windows10Posture -ceq "EnterpriseLTSC2021") {
            if ($ProductName -notmatch '(?i)^Windows 10 Enterprise LTSC 2021$' -or $BuildNumber -ne 19044) {
                throw "strict Windows 10 posture requires Enterprise LTSC 2021 build 19044"
            }
            return "windows10-enterprise-ltsc-2021-mainstream"
        }
        if ([string]::IsNullOrWhiteSpace($SupportDecisionReference) -or
            $SupportDecisionReference.Length -gt 256 -or
            $SupportDecisionReference.Contains("`n") -or
            $SupportDecisionReference.Contains("`r")) {
            throw "ApprovedException requires a concise recorded product-decision reference"
        }
        Assert-ProbeEvidenceValueSafe -Value $SupportDecisionReference -PropertyName "supportDecisionReference"
        return "windows10-explicit-product-exception"
    }

    if ($BuildNumber -lt 22000) {
        throw "Windows 11 row requires build 22000 or later, got $BuildNumber"
    }
    if ($BuildNumber -lt 26100) {
        throw "Windows 11 row requires a currently serviced 24H2-or-later build (26100+), got $BuildNumber"
    }
    "windows11-currently-serviced-operator-attested"
}

function Assert-ProbeAudioEndpointPlan {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$OutputEndpointName,
        [Parameter(Mandatory = $true)][string]$DefaultInputName,
        [Parameter(Mandatory = $true)][string]$SelectedInputName,
        [Parameter(Mandatory = $true)][bool]$SingleInputApprovedException,
        [string]$SingleInputDecisionReference = ""
    )

    if ([string]::IsNullOrWhiteSpace($OutputEndpointName) -or
        [string]::IsNullOrWhiteSpace($DefaultInputName) -or
        [string]::IsNullOrWhiteSpace($SelectedInputName) -or
        $OutputEndpointName -ieq $DefaultInputName -or
        $OutputEndpointName -ieq $SelectedInputName) {
        throw "output and input endpoint names must be non-empty and physically distinct"
    }
    if ($SingleInputApprovedException) {
        if ($DefaultInputName -ine $SelectedInputName) {
            throw "single-input exception requires selected input to equal the default physical input"
        }
        if ([string]::IsNullOrWhiteSpace($SingleInputDecisionReference) -or
            $SingleInputDecisionReference.Length -gt 256 -or
            $SingleInputDecisionReference.Contains("`n") -or
            $SingleInputDecisionReference.Contains("`r")) {
            throw "single-input exception requires a concise recorded owner-decision reference"
        }
        Assert-ProbeEvidenceValueSafe `
            -Value $SingleInputDecisionReference `
            -PropertyName "singleInputDecisionReference"
        return "single-input-owner-approved"
    }
    if ($DefaultInputName -ieq $SelectedInputName) {
        throw "default and selected inputs must identify distinct physical endpoints without an approved exception"
    }
    if (-not [string]::IsNullOrWhiteSpace($SingleInputDecisionReference)) {
        throw "single-input decision reference requires -SingleInputApprovedException"
    }
    "distinct-default-and-selected-inputs"
}

function Assert-ProbeScenarioVerdictForInputPosture {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)]
        [ValidateSet("distinct-default-and-selected-inputs", "single-input-owner-approved")]
        [string]$InputPosture,
        [Parameter(Mandatory = $true)][ValidateSet("H00", "H01", "H02", "H03", "H04", "H05", "H06", "H07", "H08", "H09", "H10", "H11", "H12", "H13", "H14", "H15", "H16", "H17")][string]$Scenario,
        [Parameter(Mandatory = $true)][ValidateSet("PASS", "FAIL", "BLOCKED")][string]$Verdict
    )

    if ($InputPosture -ceq "single-input-owner-approved" -and
        $Scenario -cin @("H04", "H08", "H12") -and
        $Verdict -ceq "PASS") {
        throw "$Scenario cannot PASS under the single-input exception; record BLOCKED with the distinct-device next action"
    }
}

function Get-ProbeHardwareHostEvidence {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][ValidateSet("windows10", "windows11")][string]$OSFamily,
        [Parameter(Mandatory = $true)][bool]$PhysicalMachineAttested,
        [Parameter(Mandatory = $true)][bool]$ConsoleOperatorAttested,
        [Parameter(Mandatory = $true)][string]$OutputEndpointName,
        [Parameter(Mandatory = $true)][string]$DefaultInputName,
        [Parameter(Mandatory = $true)][string]$SelectedInputName,
        [bool]$SingleInputApprovedException = $false,
        [string]$SingleInputDecisionReference = "",
        [ValidateSet("EnterpriseLTSC2021", "ApprovedException")][string]$Windows10Posture = "EnterpriseLTSC2021",
        [string]$SupportDecisionReference = ""
    )

    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
        throw "hardware evidence host collection is Windows-only"
    }
    $InputPosture = Assert-ProbeAudioEndpointPlan `
        -OutputEndpointName $OutputEndpointName `
        -DefaultInputName $DefaultInputName `
        -SelectedInputName $SelectedInputName `
        -SingleInputApprovedException $SingleInputApprovedException `
        -SingleInputDecisionReference $SingleInputDecisionReference

    $CurrentVersion = Get-ItemProperty "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion"
    $OperatingSystem = Get-CimInstance Win32_OperatingSystem
    $ComputerSystem = Get-CimInstance Win32_ComputerSystem
    $DeveloperMode = 0
    $DeveloperModeProperty = Get-ItemProperty `
        "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock" `
        -Name AllowDevelopmentWithoutDevLicense `
        -ErrorAction SilentlyContinue
    if ($null -ne $DeveloperModeProperty) {
        $DeveloperMode = [int]$DeveloperModeProperty.AllowDevelopmentWithoutDevLicense
    }
    $BuildNumber = [int]$CurrentVersion.CurrentBuildNumber
    $SupportPosture = Assert-ProbeHostIdentity `
        -OSFamily $OSFamily `
        -ProductName ([string]$CurrentVersion.ProductName) `
        -BuildNumber $BuildNumber `
        -Architecture ([string]$env:PROCESSOR_ARCHITECTURE) `
        -Manufacturer ([string]$ComputerSystem.Manufacturer) `
        -Model ([string]$ComputerSystem.Model) `
        -DeveloperModeValue $DeveloperMode `
        -PhysicalMachineAttested $PhysicalMachineAttested `
        -ConsoleOperatorAttested $ConsoleOperatorAttested `
        -Windows10Posture $Windows10Posture `
        -SupportDecisionReference $SupportDecisionReference

    $AudioEndpoints = @()
    if ($null -ne (Get-Command Get-PnpDevice -ErrorAction SilentlyContinue)) {
        $AudioEndpoints = @(
            Get-PnpDevice -Class AudioEndpoint -PresentOnly -ErrorAction Stop |
                Sort-Object FriendlyName, InstanceId |
                ForEach-Object {
                    [ordered]@{
                        friendlyName = [string]$_.FriendlyName
                        instanceId = [string]$_.InstanceId
                        status = [string]$_.Status
                    }
                }
        )
    }
    if (@($AudioEndpoints | Where-Object { $_.friendlyName -ieq $DefaultInputName }).Count -eq 0) {
        throw "default input '$DefaultInputName' is not present in AudioEndpoint inventory"
    }
    if (@($AudioEndpoints | Where-Object { $_.friendlyName -ieq $SelectedInputName }).Count -eq 0) {
        throw "selected input '$SelectedInputName' is not present in AudioEndpoint inventory"
    }
    if (@($AudioEndpoints | Where-Object { $_.friendlyName -ieq $OutputEndpointName }).Count -eq 0) {
        throw "output endpoint '$OutputEndpointName' is not present in AudioEndpoint inventory"
    }

    $AudioDrivers = @(
        Get-CimInstance Win32_PnPSignedDriver |
            Where-Object { [string]$_.DeviceClass -ieq "MEDIA" } |
            Sort-Object DeviceName, DeviceID |
            ForEach-Object {
                [ordered]@{
                    deviceName = [string]$_.DeviceName
                    deviceId = [string]$_.DeviceID
                    manufacturer = [string]$_.Manufacturer
                    driverProvider = [string]$_.DriverProviderName
                    driverVersion = [string]$_.DriverVersion
                    driverDateUTC = if ($null -ne $_.DriverDate) {
                        ([DateTime]$_.DriverDate).ToUniversalTime().ToString("o")
                    } else { $null }
                }
            }
    )

    $WACKPath = Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\App Certification Kit\appcert.exe"
    if (-not (Test-Path -LiteralPath $WACKPath -PathType Leaf)) {
        throw "Windows App Certification Kit appcert.exe is required before the hardware run"
    }
    $WACKVersion = (Get-Item -LiteralPath $WACKPath).VersionInfo.FileVersion
    $Hotfixes = @(
        Get-HotFix -ErrorAction Stop |
            Sort-Object InstalledOn -Descending |
            Select-Object -First 10 |
            ForEach-Object {
                [ordered]@{
                    hotFixId = [string]$_.HotFixID
                    installedOn = if ($null -ne $_.InstalledOn) {
                        ([DateTime]$_.InstalledOn).ToString("yyyy-MM-dd")
                    } else { $null }
                }
            }
    )

    [ordered]@{
        schemaVersion = 1
        verificationBoundary = "operator-attested-physical-host-inventory; not scenario pass evidence"
        collectedAtUTC = [DateTime]::UtcNow.ToString("o")
        osFamily = $OSFamily
        productName = [string]$CurrentVersion.ProductName
        editionId = [string]$CurrentVersion.EditionID
        displayVersion = [string]$CurrentVersion.DisplayVersion
        currentBuild = [string]$CurrentVersion.CurrentBuildNumber
        ubr = [int]$CurrentVersion.UBR
        fullBuild = "$($CurrentVersion.CurrentBuildNumber).$($CurrentVersion.UBR)"
        osArchitecture = [string]$OperatingSystem.OSArchitecture
        processArchitecture = [string]$env:PROCESSOR_ARCHITECTURE
        installationType = [string]$CurrentVersion.InstallationType
        supportPosture = $SupportPosture
        supportDecisionReference = if ([string]::IsNullOrWhiteSpace($SupportDecisionReference)) { $null } else { $SupportDecisionReference }
        physicalMachineAttested = $PhysicalMachineAttested
        physicalConsoleOperatorAttested = $ConsoleOperatorAttested
        manufacturer = [string]$ComputerSystem.Manufacturer
        model = [string]$ComputerSystem.Model
        hypervisorPresent = [bool]$ComputerSystem.HypervisorPresent
        developerMode = $false
        timezone = [TimeZoneInfo]::Local.Id
        outputEndpointName = $OutputEndpointName
        defaultInputName = $DefaultInputName
        selectedInputName = $SelectedInputName
        inputPosture = $InputPosture
        singleInputDecisionReference = if ([string]::IsNullOrWhiteSpace($SingleInputDecisionReference)) { $null } else { $SingleInputDecisionReference }
        audioEndpoints = @($AudioEndpoints)
        audioDrivers = @($AudioDrivers)
        recentHotfixes = @($Hotfixes)
        wackVersion = [string]$WACKVersion
    }
}

function Test-ProbeHotkeyAvailability {
    [CmdletBinding()]
    param()

    $Registration = [Pulsar.ProbeHotkeyRegistration]::new()
    try {
        [pscustomobject]@{
            Registered = [bool]$Registration.Registered
            Win32Error = [int]$Registration.Win32Error
        }
    } finally {
        $Registration.Dispose()
    }
}

function Test-ProbePickerFixtureExclusiveAccess {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [switch]$Delete
    )

    $Resolved = (Resolve-Path -LiteralPath $Path).Path
    $Stream = [IO.File]::Open($Resolved, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    try {
        $Length = $Stream.Length
    } finally {
        $Stream.Dispose()
    }
    $Temporary = "$Resolved.cleanup-$([guid]::NewGuid().ToString('N'))"
    $Deleted = $false
    try {
        Move-Item -LiteralPath $Resolved -Destination $Temporary
        if ($Delete) {
            Remove-Item -LiteralPath $Temporary -Force
            $Deleted = -not (Test-Path -LiteralPath $Temporary) -and -not (Test-Path -LiteralPath $Resolved)
            if (-not $Deleted) {
                throw "picker fixture did not become deletable after exclusive open"
            }
        } else {
            Move-Item -LiteralPath $Temporary -Destination $Resolved
        }
    } finally {
        if ((Test-Path -LiteralPath $Temporary) -and -not (Test-Path -LiteralPath $Resolved)) {
            Move-Item -LiteralPath $Temporary -Destination $Resolved -ErrorAction SilentlyContinue
        }
    }
    [pscustomobject]@{ Accessible = $true; SizeBytes = [int64]$Length; Deleted = $Deleted }
}
