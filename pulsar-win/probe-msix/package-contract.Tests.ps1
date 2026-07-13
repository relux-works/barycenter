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

Write-Host "Frozen package identity, package family, declarations, and capability regressions passed."
