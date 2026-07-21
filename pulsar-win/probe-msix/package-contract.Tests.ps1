$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "package-contract.ps1")

$Source = Get-Content (Join-Path $PSScriptRoot "AppxManifest.xml.in") -Raw
$Contract = Get-ProbeManifestTemplateContract
$ExpectedFamily = "ReluxWorksLLC.PulsarBarycenter_q036g2bzd7ngc"
if ($Contract.PackageFamilyName -cne $ExpectedFamily) {
    throw "Publisher-derived package family is '$($Contract.PackageFamilyName)', expected '$ExpectedFamily'"
}

$Mutations = [ordered]@{
    "identity" = $Source.Replace(
        'Name="ReluxWorksLLC.PulsarBarycenter"',
        'Name="ReluxWorksLLC.PulsarProbe"'
    )
    "target family" = $Source.Replace(
        'MinVersion="10.0.19041.0"',
        'MinVersion="10.0.17763.0"'
    )
    "application extension" = $Source.Replace(
        '    </Application>',
        '      <Extensions><uap:Extension Category="windows.appService" EntryPoint="Unexpected" /></Extensions>' + "`n" + '    </Application>'
    )
    "capability" = $Source.Replace(
        '    <DeviceCapability Name="microphone" />',
        '    <DeviceCapability Name="microphone" />' + "`n" + '    <Capability Name="runFullTrust" />'
    )
}

foreach ($Name in $Mutations.Keys) {
    if ($Mutations[$Name] -ceq $Source) {
        throw "the $Name negative fixture did not mutate the manifest"
    }
    [xml]$Manifest = $Mutations[$Name]
    $Rejected = $false
    try {
        $null = Assert-ProbeManifestContract -Manifest $Manifest
    } catch {
        $Rejected = $true
    }
    if (-not $Rejected) {
        throw "the frozen manifest contract accepted the $Name negative fixture"
    }
}

$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$RecordingCue = Join-Path $RepoRoot "assets\audio\pulsar-recording-cue.wav"
$ExpectedCueSHA256 = "479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd"
if (-not (Test-Path $RecordingCue)) {
    throw "canonical recording cue source is missing"
}
$CueHash = (Get-FileHash $RecordingCue -Algorithm SHA256).Hash.ToLowerInvariant()
if ($CueHash -cne $ExpectedCueSHA256) {
    throw "canonical recording cue digest is '$CueHash', expected '$ExpectedCueSHA256'"
}

$PETemp = Join-Path ([IO.Path]::GetTempPath()) ("pulsar-pe-contract-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $PETemp | Out-Null
try {
    function Write-TestPEImage {
        param(
            [Parameter(Mandatory = $true)][string]$Path,
            [Parameter(Mandatory = $true)][UInt16]$Subsystem
        )
        $Bytes = [byte[]]::new(256)
        [BitConverter]::GetBytes([UInt16]0x5A4D).CopyTo($Bytes, 0)
        [BitConverter]::GetBytes([Int32]0x80).CopyTo($Bytes, 0x3C)
        [BitConverter]::GetBytes([UInt32]0x00004550).CopyTo($Bytes, 0x80)
        [BitConverter]::GetBytes([UInt16]0x020B).CopyTo($Bytes, 0x80 + 24)
        [BitConverter]::GetBytes($Subsystem).CopyTo($Bytes, 0x80 + 24 + 68)
        [IO.File]::WriteAllBytes($Path, $Bytes)
    }

    $GUIFixture = Join-Path $PETemp "gui.exe"
    Write-TestPEImage -Path $GUIFixture -Subsystem 2
    $GUIContract = Assert-ProbeGUIExecutable -Path $GUIFixture
    if ($GUIContract.Subsystem -ne 2 -or $GUIContract.Name -cne "windows-gui") {
        throw "GUI executable contract returned unexpected metadata"
    }

    $ConsoleFixture = Join-Path $PETemp "console.exe"
    Write-TestPEImage -Path $ConsoleFixture -Subsystem 3
    $ConsoleRejected = $false
    try {
        $null = Assert-ProbeGUIExecutable -Path $ConsoleFixture
    } catch {
        $ConsoleRejected = $true
    }
    if (-not $ConsoleRejected) {
        throw "GUI executable contract accepted a console-subsystem image"
    }
} finally {
    Remove-Item -LiteralPath $PETemp -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host "Frozen package identity, declarations, GUI subsystem, capabilities, and recording cue regressions passed."
