[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$BuildRoot,
    [Parameter(Mandatory = $true)][string]$OutputDirectory
)

$ErrorActionPreference = "Stop"
$Publisher = "CN=60105954-A0D9-4E89-B32D-18AF2F423ABE"
$BuildRoot = (Resolve-Path $BuildRoot).Path
$Stage = Join-Path $BuildRoot "stage"
$PackageStage = Join-Path $BuildRoot "msix-stage"
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$Sdk = Get-ChildItem "C:\Program Files (x86)\Windows Kits\10\bin" -Directory |
    Where-Object { (Test-Path "$($_.FullName)\x64\makeappx.exe") -and (Test-Path "$($_.FullName)\x64\signtool.exe") } |
    Sort-Object Name -Descending | Select-Object -First 1
if (-not $Sdk) { throw "Windows SDK packaging tools not found" }
$MakeAppx = "$($Sdk.FullName)\x64\makeappx.exe"
$SignTool = "$($Sdk.FullName)\x64\signtool.exe"

Remove-Item -Recurse -Force $PackageStage -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $PackageStage, (Join-Path $PackageStage "Assets"), $OutputDirectory | Out-Null
Copy-Item (Join-Path $Stage "*.dll") $PackageStage
Copy-Item (Join-Path $Stage "pulsar-codec-probe.exe") $PackageStage
$Dlls = @(Get-ChildItem $PackageStage -File -Filter "*.dll")
if ($Dlls.Count -ne 5) { throw "package must contain exactly four FFmpeg DLLs and one bridge DLL" }
$RequiredDllPatterns = @("avformat-*.dll", "avcodec-*.dll", "avutil-*.dll", "swresample-*.dll", "pulsar_codec_bridge.dll")
foreach ($Pattern in $RequiredDllPatterns) {
    if (@(Get-ChildItem $PackageStage -File -Filter $Pattern).Count -ne 1) { throw "missing or duplicate allowlisted DLL: $Pattern" }
}
if (Test-Path (Join-Path $PackageStage "ff*.exe")) { throw "FFmpeg program must not be packaged" }
$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Copy-Item (Join-Path $RepoRoot "pulsar-win\msix\Assets\*.png") (Join-Path $PackageStage "Assets")

$Manifest = @"
<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"
 xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10"
 xmlns:uap10="http://schemas.microsoft.com/appx/manifest/uap/windows10/10"
 IgnorableNamespaces="uap uap10">
 <Identity Name="ReluxWorksLLC.PulsarCodecProbe" Publisher="$Publisher" Version="0.1.0.0" ProcessorArchitecture="x64" />
 <Properties><DisplayName>Pulsar Codec Probe</DisplayName><PublisherDisplayName>Relux Works, LLC</PublisherDisplayName><Logo>Assets\StoreLogo.png</Logo></Properties>
 <Dependencies><TargetDeviceFamily Name="Windows.Desktop" MinVersion="10.0.19041.0" MaxVersionTested="10.0.26100.0" /></Dependencies>
 <Resources><Resource Language="en-US" /></Resources>
 <Applications><Application Id="PulsarCodecProbe" Executable="pulsar-codec-probe.exe" uap10:TrustLevel="appContainer" uap10:RuntimeBehavior="packagedClassicApp"><uap:VisualElements DisplayName="Pulsar Codec Probe" Description="Offline bundled decoder engineering probe" BackgroundColor="#12103a" Square150x150Logo="Assets\Square150x150Logo.png" Square44x44Logo="Assets\Square44x44Logo.png" /></Application></Applications>
</Package>
"@
$Manifest | Set-Content -Encoding utf8 (Join-Path $PackageStage "AppxManifest.xml")

$Certificate = New-SelfSignedCertificate -Type Custom -Subject $Publisher -FriendlyName "Pulsar codec probe CI signer" `
    -CertStoreLocation "Cert:\CurrentUser\My" -KeyAlgorithm RSA -KeyLength 2048 -HashAlgorithm SHA256 `
    -KeyUsage DigitalSignature -KeyExportPolicy NonExportable -NotAfter (Get-Date).AddDays(2) `
    -TextExtension @("2.5.29.37={text}1.3.6.1.5.5.7.3.3", "2.5.29.19={text}")
$CertificateFile = Join-Path $BuildRoot "codec-probe-ci-signer.cer"
$null = Export-Certificate -Cert $Certificate -FilePath $CertificateFile
$TrustedCertificate = Import-Certificate -FilePath $CertificateFile -CertStoreLocation "Cert:\CurrentUser\TrustedPeople"
$InstalledPackage = $null
try {
    $Binaries = @(Get-ChildItem $PackageStage -File | Where-Object { $_.Extension -in ".dll", ".exe" })
    foreach ($Binary in $Binaries) {
        & $SignTool sign /fd SHA256 /s My /sha1 $Certificate.Thumbprint $Binary.FullName
        if ($LASTEXITCODE -ne 0) { throw "nested Authenticode signing failed: $($Binary.Name)" }
    }
    $Package = Join-Path $OutputDirectory "PulsarCodecProbe-0.1.0.0-x64-test-signed.msix"
    & $MakeAppx pack /d $PackageStage /p $Package /o
    if ($LASTEXITCODE -ne 0) { throw "MakeAppx failed" }
    & $SignTool sign /fd SHA256 /s My /sha1 $Certificate.Thumbprint $Package
    if ($LASTEXITCODE -ne 0) { throw "MSIX signing failed" }
    & $SignTool verify /pa /all $Package
    if ($LASTEXITCODE -ne 0) { throw "MSIX signature verification failed" }

    Add-AppxPackage -Path $Package
    $InstalledPackage = Get-AppxPackage -Name "ReluxWorksLLC.PulsarCodecProbe" | Select-Object -First 1
    if (-not $InstalledPackage) { throw "test-signed codec MSIX did not install" }
    $InstalledDriver = Join-Path $InstalledPackage.InstallLocation "pulsar-codec-probe.exe"
    $OfflineFixture = Join-Path $RepoRoot "acceptance\codec-spike\fixtures\smoke-v1\mp3_cbr_12s.mp3"
    $DecodeJSON = & $InstalledDriver $OfflineFixture
    if ($LASTEXITCODE -ne 0) { throw "installed offline decode failed" }
    $Decode = $DecodeJSON | ConvertFrom-Json
    if ($Decode.codec -cne "mp3" -or -not $Decode.drained -or $Decode.frames -le 0 -or
        $Decode.peakRSSBytes -le 0 -or $Decode.peakRSSBytes -gt 268435456) {
        throw "installed offline decode returned invalid evidence"
    }

    $Files = foreach ($File in Get-ChildItem $PackageStage -File -Recurse | Sort-Object FullName) {
        $Signature = if ($File.Extension -in ".dll", ".exe") { (Get-AuthenticodeSignature $File.FullName).Status.ToString() } else { "not-applicable" }
        [ordered]@{ path = $File.FullName.Substring($PackageStage.Length + 1).Replace("\", "/"); bytes = $File.Length; sha256 = (Get-FileHash $File.FullName -Algorithm SHA256).Hash.ToLowerInvariant(); signature = $Signature }
    }
    [ordered]@{
        schemaVersion = 1; contract = "p2-bundled-ffmpeg-probe.v1"; platform = "windows-amd64";
        package = [IO.Path]::GetFileName($Package); packageBytes = (Get-Item $Package).Length;
        packageSha256 = (Get-FileHash $Package -Algorithm SHA256).Hash.ToLowerInvariant();
        engineeringSignature = "ephemeral-ci-test-certificate"; releaseSignature = "not-proven";
        runtimeExecutableDownload = $false; decoderProcessOwnsNetwork = $false; files = @($Files);
        offlineInstalledDecode = $true; installedDecode = $Decode;
        shippingDecision = "rejected-until-all-required-platform-and-release-evidence-exists";
        claimClass = "repository-engineering-prototype"
    } | ConvertTo-Json -Depth 8 | Set-Content -Encoding utf8 (Join-Path $OutputDirectory "receipt-windows-amd64.json")
} finally {
    if ($InstalledPackage) { Remove-AppxPackage -Package $InstalledPackage.PackageFullName -ErrorAction SilentlyContinue }
    Remove-Item -Force "Cert:\CurrentUser\My\$($Certificate.Thumbprint)" -ErrorAction SilentlyContinue
    Remove-Item -Force "Cert:\CurrentUser\TrustedPeople\$($TrustedCertificate.Thumbprint)" -ErrorAction SilentlyContinue
    Remove-Item -Force $CertificateFile -ErrorAction SilentlyContinue
}
