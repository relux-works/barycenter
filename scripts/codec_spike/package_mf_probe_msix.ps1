[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$BuildRoot,
    [Parameter(Mandatory = $true)][string]$OutputDirectory,
    [ValidateRange(0, 7200)][int]$SoakSeconds = 60
)

$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$Contract = Get-Content (Join-Path $RepoRoot "acceptance\codec-spike\media-foundation-probe-v1.json") -Raw | ConvertFrom-Json
$Publisher = $Contract.distributionBaseline.publisher
$Identity = $Contract.distributionBaseline.packageIdentity
$BuildRoot = [IO.Path]::GetFullPath($BuildRoot)
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Force -Path $BuildRoot, $OutputDirectory | Out-Null

$Sdk = Get-ChildItem "C:\Program Files (x86)\Windows Kits\10\bin" -Directory |
    Where-Object { (Test-Path "$($_.FullName)\x64\makeappx.exe") -and (Test-Path "$($_.FullName)\x64\signtool.exe") } |
    Sort-Object Name -Descending | Select-Object -First 1
if (-not $Sdk) { throw "Windows SDK packaging tools not found" }
$MakeAppx = "$($Sdk.FullName)\x64\makeappx.exe"
$SignTool = "$($Sdk.FullName)\x64\signtool.exe"

$VsWhere = "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe"
$VsInstall = & $VsWhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
if (-not $VsInstall) { throw "Visual Studio C++ toolchain not found" }
$DumpBin = Get-ChildItem (Join-Path $VsInstall "VC\Tools\MSVC") -Recurse -Filter dumpbin.exe |
    Where-Object { $_.FullName -match 'Hostx64\\x64\\dumpbin.exe$' } |
    Sort-Object FullName -Descending | Select-Object -First 1
if (-not $DumpBin) { throw "dumpbin.exe not found" }

$Certificate = New-SelfSignedCertificate -Type Custom -Subject $Publisher -FriendlyName "Pulsar Media Foundation probe CI signer" `
    -CertStoreLocation "Cert:\CurrentUser\My" -KeyAlgorithm RSA -KeyLength 2048 -HashAlgorithm SHA256 `
    -KeyUsage DigitalSignature -KeyExportPolicy NonExportable -NotAfter (Get-Date).AddDays(2) `
    -TextExtension @("2.5.29.37={text}1.3.6.1.5.5.7.3.3", "2.5.29.19={text}")
$CertificateFile = Join-Path $BuildRoot "mf-probe-ci-signer.cer"
$null = Export-Certificate -Cert $Certificate -FilePath $CertificateFile
$TrustedCertificate = Import-Certificate -FilePath $CertificateFile -CertStoreLocation "Cert:\LocalMachine\TrustedPeople"
$InstalledPackage = $null

$Receipts = @()
try {
    foreach ($Target in @(
        [pscustomobject]@{ Id = "windows-amd64"; CMakeArch = "x64"; ManifestArch = "x64"; Runnable = $true },
        [pscustomobject]@{ Id = "windows-arm64"; CMakeArch = "ARM64"; ManifestArch = "arm64"; Runnable = $false }
    )) {
        $NativeBuild = Join-Path $BuildRoot "native-$($Target.ManifestArch)"
        $Stage = Join-Path $BuildRoot "stage-$($Target.ManifestArch)"
        Remove-Item -Recurse -Force $NativeBuild, $Stage -ErrorAction SilentlyContinue
        New-Item -ItemType Directory -Force -Path $NativeBuild, $Stage, (Join-Path $Stage "Assets"), (Join-Path $Stage "Fixtures") | Out-Null
        & cmake -S (Join-Path $PSScriptRoot "native") -B $NativeBuild -A $Target.CMakeArch
        if ($LASTEXITCODE -ne 0) { throw "CMake configure failed for $($Target.Id)" }
        & cmake --build $NativeBuild --config Release --parallel
        if ($LASTEXITCODE -ne 0) { throw "CMake build failed for $($Target.Id)" }
        $Executable = Join-Path $NativeBuild "Release\PulsarMediaFoundationProbe.exe"
        if (-not (Test-Path $Executable)) { throw "native probe missing for $($Target.Id)" }
        Copy-Item $Executable (Join-Path $Stage "PulsarMediaFoundationProbe.exe")
        Copy-Item (Join-Path $RepoRoot "pulsar-win\msix\Assets\*.png") (Join-Path $Stage "Assets")
        foreach ($Fixture in $Contract.smokeFixtures) {
            Copy-Item (Join-Path $RepoRoot $Fixture.path) (Join-Path $Stage "Fixtures")
        }

        $Manifest = @"
<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"
 xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10"
 xmlns:uap10="http://schemas.microsoft.com/appx/manifest/uap/windows10/10"
 IgnorableNamespaces="uap uap10">
 <Identity Name="$Identity" Publisher="$Publisher" Version="0.1.0.0" ProcessorArchitecture="$($Target.ManifestArch)" />
 <Properties><DisplayName>Pulsar Media Foundation Probe</DisplayName><PublisherDisplayName>Relux Works, LLC</PublisherDisplayName><Logo>Assets\StoreLogo.png</Logo></Properties>
 <Dependencies><TargetDeviceFamily Name="Windows.Desktop" MinVersion="10.0.19041.0" MaxVersionTested="10.0.26100.0" /></Dependencies>
 <Resources><Resource Language="en-US" /></Resources>
 <Applications><Application Id="PulsarMediaFoundationProbe" Executable="PulsarMediaFoundationProbe.exe" uap10:TrustLevel="appContainer" uap10:RuntimeBehavior="packagedClassicApp"><uap:VisualElements DisplayName="Pulsar Media Foundation Probe" Description="Offline inbox decoder engineering probe" BackgroundColor="#12103a" Square150x150Logo="Assets\Square150x150Logo.png" Square44x44Logo="Assets\Square44x44Logo.png" /></Application></Applications>
</Package>
"@
        $Manifest | Set-Content -Encoding utf8 (Join-Path $Stage "AppxManifest.xml")
        $ManifestText = Get-Content (Join-Path $Stage "AppxManifest.xml") -Raw
        if ($ManifestText -notmatch 'uap10:TrustLevel="appContainer"' -or
            $ManifestText -match 'runFullTrust|<Capabilities>') {
            throw "manifest weakened the AppContainer or capability posture"
        }

        $StagedExecutable = Join-Path $Stage "PulsarMediaFoundationProbe.exe"
        & $SignTool sign /fd SHA256 /s My /sha1 $Certificate.Thumbprint $StagedExecutable
        if ($LASTEXITCODE -ne 0) { throw "nested executable signing failed for $($Target.Id)" }
        $Headers = & $DumpBin.FullName /headers $StagedExecutable | Out-String
        if ($Headers -notmatch '(?im)^\s+App Container\s*$') { throw "PE AppContainer flag missing for $($Target.Id)" }
        $ImportsText = & $DumpBin.FullName /imports $StagedExecutable | Out-String
        $Imports = @($ImportsText -split "`r?`n" | ForEach-Object { $_.Trim().ToLowerInvariant() } |
            Where-Object { $_ -match '^[a-z0-9_.-]+\.dll$' } | Sort-Object -Unique)
        $AllowedImports = @("advapi32.dll", "kernel32.dll", "kernelbase.dll", "mfplat.dll", "mfreadwrite.dll", "ole32.dll", "psapi.dll")
        $UnknownImports = @($Imports | Where-Object {
            $_ -notin $AllowedImports -and -not $_.StartsWith("api-ms-win-") -and -not $_.StartsWith("ext-ms-win-")
        })
        if ($UnknownImports.Count -ne 0) {
            throw "unexpected non-system imports for $($Target.Id): $($UnknownImports -join ', ')"
        }

        $Package = Join-Path $OutputDirectory "PulsarMediaFoundationProbe-0.1.0.0-$($Target.ManifestArch)-test-signed.msix"
        & $MakeAppx pack /d $Stage /p $Package /o
        if ($LASTEXITCODE -ne 0) { throw "MakeAppx failed for $($Target.Id)" }
        & $SignTool sign /fd SHA256 /s My /sha1 $Certificate.Thumbprint $Package
        if ($LASTEXITCODE -ne 0) { throw "MSIX signing failed for $($Target.Id)" }
        $Signature = Get-AuthenticodeSignature $Package
        if (-not $Signature.SignerCertificate -or $Signature.SignerCertificate.Thumbprint -cne $Certificate.Thumbprint) {
            throw "MSIX embedded signer mismatch for $($Target.Id)"
        }
        $Receipts += [ordered]@{
            platform = $Target.Id
            package = [IO.Path]::GetFileName($Package)
            packageBytes = (Get-Item $Package).Length
            packageSha256 = (Get-FileHash $Package -Algorithm SHA256).Hash.ToLowerInvariant()
            executableSha256 = (Get-FileHash $StagedExecutable -Algorithm SHA256).Hash.ToLowerInvariant()
            nestedSignature = (Get-AuthenticodeSignature $StagedExecutable).Status.ToString()
            peAppContainer = $true
            imports = $Imports
            manifestCapabilities = @()
            runFullTrust = $false
            runnableOnHostedRunner = $Target.Runnable
        }
    }

    $X64Receipt = $Receipts | Where-Object { $_.platform -ceq "windows-amd64" }
    $X64Package = Join-Path $OutputDirectory $X64Receipt.package
    Add-AppxPackage -Path $X64Package
    $InstalledPackage = Get-AppxPackage -Name $Identity | Select-Object -First 1
    if (-not $InstalledPackage) { throw "test-signed Media Foundation MSIX did not install" }
    $InstalledExecutable = Join-Path $InstalledPackage.InstallLocation "PulsarMediaFoundationProbe.exe"
    $LocalState = Join-Path $env:LOCALAPPDATA "Packages\$($InstalledPackage.PackageFamilyName)\LocalState"
    New-Item -ItemType Directory -Force -Path $LocalState | Out-Null
    $EvidencePath = Join-Path $LocalState "mf-probe-evidence.json"
    Remove-Item -Force $EvidencePath -ErrorAction SilentlyContinue
    $Direct = Start-Process -FilePath $InstalledExecutable -ArgumentList "--soak-seconds=0" -Wait -PassThru -ErrorAction Stop
    if ($Direct.ExitCode -ne 0 -or -not (Test-Path $EvidencePath)) {
        throw "installed-path AppContainer launch failed its self-check"
    }
    $DirectEvidence = Get-Content $EvidencePath -Raw | ConvertFrom-Json
    if (-not $DirectEvidence.tokenIsAppContainer -or $DirectEvidence.packageFamilyName -cne $InstalledPackage.PackageFamilyName) {
        throw "installed-path launch did not retain AppContainer package identity"
    }
    $Aumid = "$($InstalledPackage.PackageFamilyName)!PulsarMediaFoundationProbe"
    $Activation = & (Join-Path $PSScriptRoot "activate_mf_probe.ps1") `
        -ApplicationUserModelId $Aumid -EvidencePath $EvidencePath -SoakSeconds $SoakSeconds
    $Evidence = Get-Content $EvidencePath -Raw | ConvertFrom-Json
    if ($Activation.ExitCode -ne 0 -or -not $Evidence.passed -or -not $Evidence.tokenIsAppContainer -or
        $Evidence.renderCallbackUsed -or $Evidence.decoderOwnsNetwork -or
        $Evidence.maximumPreparedReadBytes -gt 1048576 -or $Evidence.peakRSSBytes -gt 209715200) {
        throw "packaged Media Foundation evidence failed the frozen hosted gates"
    }
    if (@($Evidence.fixtures).Count -ne 6 -or @($Evidence.fixtures | Where-Object { -not $_.passed }).Count -ne 0) {
        throw "exact six-fixture Media Foundation evidence is incomplete"
    }

    [ordered]@{
        schemaVersion = 1
        contract = $Contract.contract
        claimClass = "repository-engineering-prototype"
        runner = [ordered]@{
            os = [Environment]::OSVersion.VersionString
            architecture = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
            githubRunner = $env:RUNNER_NAME
            realHardware = $false
        }
        packageIdentity = $Identity
        signer = "ephemeral-ci-test-certificate"
        releaseSignature = "not-proven"
        packages = @($Receipts)
        activation = [ordered]@{
            applicationUserModelId = $Aumid
            options = $Activation.ActivationOptions
            debugModeEnabled = $Activation.DebugModeEnabled
            processId = $Activation.ProcessId
            exitCode = $Activation.ExitCode
            directInstalledLaunchExitCode = $Direct.ExitCode
            directInstalledLaunchTokenIsAppContainer = $DirectEvidence.tokenIsAppContainer
        }
        evidence = $Evidence
        manualEvidence = [ordered]@{
            twoHourSoak = "not-run-on-hosted-runner"
            windows10 = "manual-epic"
            windows11x64 = "manual-epic"
            windows11arm64 = "manual-epic"
        }
        shippingDecision = $Contract.decision.shipping
    } | ConvertTo-Json -Depth 12 | Set-Content -Encoding utf8 (Join-Path $OutputDirectory "receipt-windows-media-foundation.json")
} finally {
    if ($InstalledPackage) { Remove-AppxPackage -Package $InstalledPackage.PackageFullName -ErrorAction SilentlyContinue }
    Remove-Item -Force "Cert:\CurrentUser\My\$($Certificate.Thumbprint)" -ErrorAction SilentlyContinue
    Remove-Item -Force "Cert:\LocalMachine\TrustedPeople\$($TrustedCertificate.Thumbprint)" -ErrorAction SilentlyContinue
    Remove-Item -Force $CertificateFile -ErrorAction SilentlyContinue
}
