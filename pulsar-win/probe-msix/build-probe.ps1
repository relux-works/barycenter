param(
    [string]$Version = "0.1.0.0",
    [string]$Configuration = "Release",
    [string]$OutputDirectory = ""
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "native-command.ps1")
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
New-Item -ItemType Directory -Force -Path $NativeBuild, $Stage, (Join-Path $Stage "Assets"), $OutputDirectory | Out-Null

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
(Get-Content (Join-Path $PSScriptRoot "AppxManifest.xml.in") -Raw).Replace("@VERSION@", $Version) |
    Set-Content -Encoding utf8 (Join-Path $Stage "AppxManifest.xml")

$RequiredPayload = @(
    (Join-Path $Stage "AppxManifest.xml"),
    (Join-Path $Stage "pulsar-win-probe-amd64.exe"),
    (Join-Path $Stage "pulsar-capture.dll")
)
foreach ($Path in $RequiredPayload) {
    if (-not (Test-Path $Path)) { throw "package payload missing $Path" }
}

$Sdk = Get-ChildItem "C:\Program Files (x86)\Windows Kits\10\bin" -Directory |
    Where-Object { Test-Path (Join-Path $_.FullName "x64\makeappx.exe") } |
    Sort-Object Name -Descending |
    Select-Object -First 1
if (-not $Sdk) { throw "makeappx.exe was not found in the Windows SDK" }
$MakeAppx = Join-Path $Sdk.FullName "x64\makeappx.exe"
$Package = Join-Path $OutputDirectory "PulsarProbe-$Version-x64.msix"
Invoke-NativeChecked -Name "MakeAppx" -Command {
    & $MakeAppx pack /d $Stage /p $Package /o
}

Write-Host "Created unsigned validation package: $Package"
Write-Host "Signed install, WACK, and Windows 10/11 hardware evidence remain explicit downstream gates."
