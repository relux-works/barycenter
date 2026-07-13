$script:ProbePackageIdentity = "ReluxWorksLLC.PulsarBarycenter"
$script:ProbePublisher = "CN=60105954-A0D9-4E89-B32D-18AF2F423ABE"
$script:ProbeApplicationID = "PulsarProbe"
$script:ProbeApplicationExecutable = "pulsar-win-probe-amd64.exe"
$script:ProbeTrustNamespace = "http://schemas.microsoft.com/appx/manifest/uap/windows10/10"
$script:ProbeExpectedCapabilities = @(
    "Capability:internetClient",
    "Capability:internetClientServer",
    "Capability:privateNetworkClientServer",
    "DeviceCapability:microphone"
)

if (-not ("Pulsar.ProbePackageIdentity" -as [type])) {
    Add-Type -TypeDefinition @"
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Text;

namespace Pulsar
{
    public static class ProbePackageIdentity
    {
        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        private struct PackageId
        {
            public UInt32 Reserved;
            public UInt32 ProcessorArchitecture;
            public UInt64 Version;
            [MarshalAs(UnmanagedType.LPWStr)] public string Name;
            [MarshalAs(UnmanagedType.LPWStr)] public string Publisher;
            public IntPtr ResourceId;
            public IntPtr PublisherId;
        }

        [DllImport("kernel32.dll", CharSet = CharSet.Unicode)]
        private static extern int PackageFamilyNameFromId(
            ref PackageId packageId,
            ref UInt32 packageFamilyNameLength,
            StringBuilder packageFamilyName);

        public static string GetFamilyName(string name, string publisher)
        {
            PackageId id = new PackageId
            {
                ProcessorArchitecture = 9,
                Name = name,
                Publisher = publisher
            };
            UInt32 length = 0;
            int result = PackageFamilyNameFromId(ref id, ref length, null);
            const int ErrorInsufficientBuffer = 122;
            if (result != ErrorInsufficientBuffer)
            {
                throw new Win32Exception(result, "PackageFamilyNameFromId sizing failed with Win32 code " + result);
            }
            StringBuilder familyName = new StringBuilder((int)length);
            result = PackageFamilyNameFromId(ref id, ref length, familyName);
            if (result != 0)
            {
                throw new Win32Exception(result, "PackageFamilyNameFromId failed with Win32 code " + result);
            }
            return familyName.ToString();
        }
    }
}
"@
}

function Get-ProbePackageFamilyName {
    [CmdletBinding()]
    param()

    [Pulsar.ProbePackageIdentity]::GetFamilyName($script:ProbePackageIdentity, $script:ProbePublisher)
}

function Assert-ProbeManifestContract {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][xml]$Manifest
    )

    $Identity = $Manifest.Package.Identity
    $Applications = @($Manifest.Package.Applications.Application)
    if ($Applications.Count -ne 1) {
        throw "probe manifest must contain exactly one Application, got $($Applications.Count)"
    }
    $Application = $Applications[0]
    $TrustLevel = $Application.GetAttribute("TrustLevel", $script:ProbeTrustNamespace)
    $RuntimeBehavior = $Application.GetAttribute("RuntimeBehavior", $script:ProbeTrustNamespace)

    $Checks = [ordered]@{
        "package identity" = @([string]$Identity.Name, $script:ProbePackageIdentity)
        "publisher" = @([string]$Identity.Publisher, $script:ProbePublisher)
        "processor architecture" = @([string]$Identity.ProcessorArchitecture, "x64")
        "application ID" = @([string]$Application.Id, $script:ProbeApplicationID)
        "application executable" = @([string]$Application.Executable, $script:ProbeApplicationExecutable)
        "trust level" = @($TrustLevel, "appContainer")
        "runtime behavior" = @($RuntimeBehavior, "packagedClassicApp")
    }
    foreach ($Name in $Checks.Keys) {
        $Actual, $Expected = $Checks[$Name]
        if ($Actual -cne $Expected) {
            throw "probe manifest $Name is '$Actual', expected '$Expected'"
        }
    }

    $DeclaredCapabilities = @(
        $Manifest.SelectNodes("//*[local-name()='Capabilities']/*") |
            ForEach-Object { "$($_.LocalName):$($_.GetAttribute('Name'))" } |
            Sort-Object
    )
    $ExpectedCapabilities = @($script:ProbeExpectedCapabilities | Sort-Object)
    $CapabilityDifference = @(Compare-Object $ExpectedCapabilities $DeclaredCapabilities)
    if ($CapabilityDifference.Count -ne 0) {
        throw "probe manifest capabilities differ from the reviewed set: $($DeclaredCapabilities -join ', ')"
    }

    [pscustomobject]@{
        PackageIdentity = [string]$Identity.Name
        Publisher = [string]$Identity.Publisher
        Version = [string]$Identity.Version
        ProcessorArchitecture = [string]$Identity.ProcessorArchitecture
        ApplicationID = [string]$Application.Id
        Executable = [string]$Application.Executable
        TrustLevel = $TrustLevel
        RuntimeBehavior = $RuntimeBehavior
        Capabilities = $DeclaredCapabilities
        PackageFamilyName = Get-ProbePackageFamilyName
    }
}

function Get-ProbeManifestTemplateContract {
    [CmdletBinding()]
    param()

    [xml]$Manifest = Get-Content (Join-Path $PSScriptRoot "AppxManifest.xml.in") -Raw
    Assert-ProbeManifestContract -Manifest $Manifest
}

function Get-ProbeSigningCertificate {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Thumbprint
    )

    $NormalizedThumbprint = ($Thumbprint -replace '\s', '').ToUpperInvariant()
    if ($NormalizedThumbprint -notmatch '^[0-9A-F]{40}$') {
        throw "signing certificate thumbprint must contain exactly 40 hexadecimal characters"
    }
    $CertificatePath = "Cert:\CurrentUser\My\$NormalizedThumbprint"
    if (-not (Test-Path $CertificatePath)) {
        throw "signing certificate was not found in Cert:\CurrentUser\My"
    }
    $Certificate = Get-Item $CertificatePath
    if ($Certificate.Subject -cne $script:ProbePublisher) {
        throw "signing certificate Subject does not exactly match the frozen manifest Publisher"
    }
    if (-not $Certificate.HasPrivateKey) {
        throw "signing certificate does not expose a private key to the current user"
    }
    $Now = Get-Date
    if ($Certificate.NotBefore -gt $Now -or $Certificate.NotAfter -le $Now) {
        throw "signing certificate is outside its validity window"
    }
    $EnhancedKeyUsages = @($Certificate.EnhancedKeyUsageList | ForEach-Object { $_.ObjectId.Value })
    if ($EnhancedKeyUsages -notcontains "1.3.6.1.5.5.7.3.3") {
        throw "signing certificate is missing the Code Signing enhanced key usage"
    }
    $Certificate
}
