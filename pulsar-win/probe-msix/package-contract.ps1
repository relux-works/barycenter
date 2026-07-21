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
            const UInt32 ProcessorArchitectureAmd64 = 9;
            PackageId id = new PackageId
            {
                ProcessorArchitecture = ProcessorArchitectureAmd64,
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

function Get-ProbeRuntimeRelativeRoot {
    [CmdletBinding()]
    param()

    # A packagedClassicApp running at appContainer trust sees LOCALAPPDATA as
    # its virtualized AC root. The Go probe writes LOCALAPPDATA\PulsarProbe;
    # host-side install, snapshot and cleanup tools reach those same bytes here.
    "Packages\$(Get-ProbePackageFamilyName)\AC\PulsarProbe"
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

    $RootDeclarations = @(
        $Manifest.Package.ChildNodes |
            Where-Object { $_.NodeType -eq [System.Xml.XmlNodeType]::Element } |
            ForEach-Object { $_.LocalName } |
            Sort-Object
    )
    $ExpectedRootDeclarations = @(
        "Applications", "Capabilities", "Dependencies", "Identity", "Properties", "Resources"
    ) | Sort-Object
    if (@(Compare-Object $ExpectedRootDeclarations $RootDeclarations).Count -ne 0) {
        throw "probe manifest root declarations differ from the frozen set: $($RootDeclarations -join ', ')"
    }
    $ApplicationDeclarations = @(
        $Application.ChildNodes |
            Where-Object { $_.NodeType -eq [System.Xml.XmlNodeType]::Element } |
            ForEach-Object { $_.LocalName }
    )
    if ($ApplicationDeclarations.Count -ne 1 -or $ApplicationDeclarations[0] -cne "VisualElements") {
        throw "probe Application may contain only the frozen VisualElements declaration"
    }

    $TargetFamilies = @($Manifest.Package.Dependencies.TargetDeviceFamily)
    if ($TargetFamilies.Count -ne 1 -or
        [string]$TargetFamilies[0].Name -cne "Windows.Desktop" -or
        [string]$TargetFamilies[0].MinVersion -cne "10.0.19041.0" -or
        [string]$TargetFamilies[0].MaxVersionTested -cne "10.0.22621.0") {
        throw "probe manifest must keep the frozen Windows.Desktop 19041/22621 target family"
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

function Get-ProbePackageManifestContract {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$PackagePath
    )

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $ResolvedPackagePath = (Resolve-Path $PackagePath).Path
    $Archive = [System.IO.Compression.ZipFile]::OpenRead($ResolvedPackagePath)
    try {
        $ManifestEntries = @($Archive.Entries | Where-Object { $_.FullName -ceq "AppxManifest.xml" })
        if ($ManifestEntries.Count -ne 1) {
            throw "MSIX must contain exactly one root AppxManifest.xml"
        }
        if ($ManifestEntries[0].Length -gt 131072) {
            throw "MSIX AppxManifest.xml exceeds the 128 KiB probe contract limit"
        }
        $Settings = [System.Xml.XmlReaderSettings]::new()
        $Settings.DtdProcessing = [System.Xml.DtdProcessing]::Prohibit
        $Settings.XmlResolver = $null
        $Settings.MaxCharactersInDocument = 131072
        $ManifestStream = $ManifestEntries[0].Open()
        try {
            $Reader = [System.Xml.XmlReader]::Create($ManifestStream, $Settings)
            try {
                $Manifest = [System.Xml.XmlDocument]::new()
                $Manifest.XmlResolver = $null
                $Manifest.Load($Reader)
            } finally {
                $Reader.Dispose()
            }
        } finally {
            $ManifestStream.Dispose()
        }
    } finally {
        $Archive.Dispose()
    }
    Assert-ProbeManifestContract -Manifest $Manifest
}

function Get-ProbeManifestTemplateContract {
    [CmdletBinding()]
    param()

    [xml]$Manifest = Get-Content (Join-Path $PSScriptRoot "AppxManifest.xml.in") -Raw
    Assert-ProbeManifestContract -Manifest $Manifest
}

function Get-WindowsExecutableSubsystem {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Path
    )

    $ResolvedPath = (Resolve-Path -LiteralPath $Path).Path
    $Stream = [IO.File]::Open($ResolvedPath, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    $Reader = [IO.BinaryReader]::new($Stream)
    try {
        if ($Stream.Length -lt 96 -or $Reader.ReadUInt16() -ne 0x5A4D) {
            throw "executable is not a valid DOS/PE image: $ResolvedPath"
        }
        $Stream.Position = 0x3C
        $PEOffset = $Reader.ReadInt32()
        if ($PEOffset -lt 0x40 -or $PEOffset + 94 -gt $Stream.Length) {
            throw "executable has an invalid PE header offset: $ResolvedPath"
        }
        $Stream.Position = $PEOffset
        if ($Reader.ReadUInt32() -ne 0x00004550) {
            throw "executable is missing the PE signature: $ResolvedPath"
        }
        $OptionalHeaderOffset = $PEOffset + 24
        $Stream.Position = $OptionalHeaderOffset
        $Magic = $Reader.ReadUInt16()
        if ($Magic -ne 0x010B -and $Magic -ne 0x020B) {
            throw "executable has unsupported optional-header magic 0x$($Magic.ToString('X4')): $ResolvedPath"
        }
        $Stream.Position = $OptionalHeaderOffset + 68
        [int]$Reader.ReadUInt16()
    } finally {
        $Reader.Dispose()
        $Stream.Dispose()
    }
}

function Assert-ProbeGUIExecutable {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Path
    )

    $Subsystem = Get-WindowsExecutableSubsystem -Path $Path
    if ($Subsystem -ne 2) {
        throw "probe executable PE subsystem is $Subsystem, expected 2 (Windows GUI): $Path"
    }
    [pscustomobject]@{
        Path = (Resolve-Path -LiteralPath $Path).Path
        Subsystem = $Subsystem
        Name = "windows-gui"
    }
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
    $EnhancedKeyUsages = @($Certificate.EnhancedKeyUsageList | ForEach-Object { [string]$_.ObjectId })
    if ($EnhancedKeyUsages -notcontains "1.3.6.1.5.5.7.3.3") {
        throw "signing certificate is missing the Code Signing enhanced key usage"
    }
    $Certificate
}
