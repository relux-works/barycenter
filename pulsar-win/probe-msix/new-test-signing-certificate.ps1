[CmdletBinding()]
param(
    [ValidateRange(1, 365)][int]$ValidityDays = 30
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "package-contract.ps1")
$Contract = Get-ProbeManifestTemplateContract

$Certificate = New-SelfSignedCertificate `
    -Type Custom `
    -Subject $Contract.Publisher `
    -FriendlyName "Pulsar signed-MSIX hardware probe" `
    -CertStoreLocation "Cert:\CurrentUser\My" `
    -KeyAlgorithm RSA `
    -KeyLength 2048 `
    -HashAlgorithm SHA256 `
    -KeyUsage DigitalSignature `
    -KeyExportPolicy NonExportable `
    -NotAfter (Get-Date).AddDays($ValidityDays) `
    -TextExtension @(
        "2.5.29.37={text}1.3.6.1.5.5.7.3.3",
        "2.5.29.19={text}"
    )

try {
    $null = Get-ProbeSigningCertificate -Thumbprint $Certificate.Thumbprint
} catch {
    Remove-Item -Force "Cert:\CurrentUser\My\$($Certificate.Thumbprint)" -ErrorAction SilentlyContinue
    throw
}
Write-Host "Created a non-exportable CurrentUser test signer for the frozen Partner Center publisher."
Write-Output $Certificate.Thumbprint
