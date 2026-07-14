param(
    [string]$Version = "0.1.0.0",
    [string]$Configuration = "Release",
    [string]$OutputDirectory = "",
    [string]$SigningCertificateThumbprint = ""
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "native-command.ps1")
. (Join-Path $PSScriptRoot "package-contract.ps1")
$PulsarRoot = Split-Path -Parent $PSScriptRoot
$RepoRoot = Split-Path -Parent $PulsarRoot
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $RepoRoot "dist\windows-probe"
}
$BuildRoot = Join-Path $PulsarRoot ".build\probe"
$NativeBuild = Join-Path $BuildRoot "native"
$Stage = Join-Path $BuildRoot "stage"

if ($Version -notmatch '^\d+\.\d+\.\d+\.\d+$') {
    throw "MSIX version must be a numeric quad, got '$Version'"
}

Remove-Item -Recurse -Force $Stage -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $NativeBuild, $Stage, (Join-Path $Stage "Assets"), (Join-Path $Stage "Assets\Audio"), $OutputDirectory | Out-Null

Invoke-NativeChecked -Name "CMake configure" -Command {
    cmake -S (Join-Path $PulsarRoot "native\pulsar-capture") -B $NativeBuild -A x64 -DBUILD_TESTING=ON
}
Invoke-NativeChecked -Name "CMake build" -Command {
    cmake --build $NativeBuild --config $Configuration --parallel
}
Invoke-NativeChecked -Name "CTest" -Command {
    ctest --test-dir $NativeBuild -C $Configuration --output-on-failure
}

Push-Location $PulsarRoot
try {
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"
    Invoke-NativeChecked -Name "go vet" -Command { go vet ./... }
    Invoke-NativeChecked -Name "go test" -Command { go test ./... }
    Invoke-NativeChecked -Name "go build" -Command {
        go build -trimpath -ldflags "-s -w -buildid= -H windowsgui" -o (Join-Path $Stage "pulsar-win-probe-amd64.exe") ./cmd/pulsar-win-probe
    }
} finally {
    Pop-Location
}

$NativeDLL = Join-Path $NativeBuild "$Configuration\pulsar-capture.dll"
if (-not (Test-Path $NativeDLL)) {
    throw "native helper was not produced at $NativeDLL"
}
Copy-Item $NativeDLL (Join-Path $Stage "pulsar-capture.dll")
Copy-Item (Join-Path $PulsarRoot "msix\Assets\*.png") (Join-Path $Stage "Assets")
$RecordingCue = Join-Path $RepoRoot "assets\audio\pulsar-recording-cue.wav"
$RecordingCueSHA256 = "479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd"
if (-not (Test-Path $RecordingCue) -or
    (Get-FileHash $RecordingCue -Algorithm SHA256).Hash.ToLowerInvariant() -cne $RecordingCueSHA256) {
    throw "canonical recording cue is missing or has an unreviewed digest"
}
Copy-Item $RecordingCue (Join-Path $Stage "Assets\Audio\pulsar-recording-cue.wav")
$RenderedManifestPath = Join-Path $Stage "AppxManifest.xml"
(Get-Content (Join-Path $PSScriptRoot "AppxManifest.xml.in") -Raw).Replace("@VERSION@", $Version) |
    Set-Content -Encoding utf8 $RenderedManifestPath
[xml]$RenderedManifest = Get-Content $RenderedManifestPath -Raw
$ManifestContract = Assert-ProbeManifestContract -Manifest $RenderedManifest

$RequiredPayload = @(
    (Join-Path $Stage "AppxManifest.xml"),
    (Join-Path $Stage "pulsar-win-probe-amd64.exe"),
    (Join-Path $Stage "pulsar-capture.dll"),
    (Join-Path $Stage "Assets\Audio\pulsar-recording-cue.wav")
)
foreach ($Path in $RequiredPayload) {
    if (-not (Test-Path $Path)) { throw "package payload missing $Path" }
}

$Sdk = Get-ChildItem "C:\Program Files (x86)\Windows Kits\10\bin" -Directory |
    Where-Object {
        (Test-Path (Join-Path $_.FullName "x64\makeappx.exe")) -and
        (Test-Path (Join-Path $_.FullName "x64\signtool.exe"))
    } |
    Sort-Object Name -Descending |
    Select-Object -First 1
if (-not $Sdk) { throw "makeappx.exe was not found in the Windows SDK" }
$MakeAppx = Join-Path $Sdk.FullName "x64\makeappx.exe"
$SignTool = Join-Path $Sdk.FullName "x64\signtool.exe"
$Signed = -not [string]::IsNullOrWhiteSpace($SigningCertificateThumbprint)
$PackageKind = if ($Signed) { "signed" } else { "unsigned" }
$Package = Join-Path $OutputDirectory "PulsarProbe-$Version-x64-$PackageKind.msix"
Invoke-NativeChecked -Name "MakeAppx" -Command {
    & $MakeAppx pack /d $Stage /p $Package /o
}

if ($Signed) {
    $SigningCertificate = Get-ProbeSigningCertificate -Thumbprint $SigningCertificateThumbprint
    Invoke-NativeChecked -Name "SignTool sign" -Command {
        & $SignTool sign /fd SHA256 /s My /sha1 $SigningCertificate.Thumbprint $Package
    }
    $EmbeddedSignature = Get-AuthenticodeSignature -FilePath $Package
    if ($null -eq $EmbeddedSignature.SignerCertificate -or
        $EmbeddedSignature.SignerCertificate.Thumbprint -cne $SigningCertificate.Thumbprint) {
        throw "signed package does not contain the selected signer certificate"
    }
}

$Hash = Get-FileHash $Package -Algorithm SHA256
$HashPath = "$Package.sha256"
"$($Hash.Hash.ToLowerInvariant())  $([IO.Path]::GetFileName($Package))" |
    Set-Content -Encoding ascii $HashPath
$MetadataPath = "$Package.json"
[ordered]@{
    schemaVersion = 1
    verificationBoundary = "package-build-and-signature-only; not installed Win10/Win11 hardware evidence"
    packageFile = [IO.Path]::GetFileName($Package)
    sha256 = $Hash.Hash.ToLowerInvariant()
    packageIdentity = $ManifestContract.PackageIdentity
    publisher = $ManifestContract.Publisher
    version = $ManifestContract.Version
    processorArchitecture = $ManifestContract.ProcessorArchitecture
    applicationId = $ManifestContract.ApplicationID
    packageFamilyName = $ManifestContract.PackageFamilyName
    applicationUserModelId = "$($ManifestContract.PackageFamilyName)!$($ManifestContract.ApplicationID)"
    trustLevel = $ManifestContract.TrustLevel
    runtimeBehavior = $ManifestContract.RuntimeBehavior
    capabilities = @($ManifestContract.Capabilities)
    recordingCue = [ordered]@{
        packagePath = "Assets/Audio/pulsar-recording-cue.wav"
        sha256 = $RecordingCueSHA256
    }
    signature = if ($Signed) { "certificate-store-sha256" } else { "unsigned-store-upload-candidate" }
    privateSigningMaterialIncluded = $false
} | ConvertTo-Json -Depth 4 | Set-Content -Encoding utf8 $MetadataPath

Write-Host "Created $PackageKind Partner Center identity probe package: $Package"
Write-Host "Frozen package family: $($ManifestContract.PackageFamilyName)"
Write-Host "Package metadata: $MetadataPath"
if ($Signed) {
    Write-Host "The package contains an embedded test signature; no private certificate material was exported."
} else {
    Write-Host "This unsigned output is only a Partner Center Store-upload candidate; it is not locally installable."
}
Write-Host "WACK and real Windows 10/11 hardware evidence remain explicit downstream gates."
