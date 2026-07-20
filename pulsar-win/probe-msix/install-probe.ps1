[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Package,
    [switch]$TrustLocalTestSigner,
    [switch]$Launch,
    [string]$ReceiptPath = ""
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "package-contract.ps1")

$PackagePath = (Resolve-Path $Package).Path
$PackageHash = (Get-FileHash -Path $PackagePath -Algorithm SHA256).Hash.ToLowerInvariant()
$ArchiveContract = Get-ProbePackageManifestContract -PackagePath $PackagePath
$Existing = @(Get-AppxPackage -Name $script:ProbePackageIdentity)
if ($Existing.Count -ne 0) {
    throw "Refusing to replace an installed package in the production Pulsar family '$script:ProbePackageIdentity'. Use a dedicated test account/host and remove the existing package first."
}

$Signature = Get-AuthenticodeSignature -FilePath $PackagePath
if ($null -eq $Signature.SignerCertificate) {
    throw "the probe package has no embedded signer certificate"
}
$Signer = $Signature.SignerCertificate
$TrustedPeoplePath = "Cert:\LocalMachine\TrustedPeople\$($Signer.Thumbprint)"
$AddedTrust = $false
$Installed = $null
$InstalledPackages = @()

try {
    if ($TrustLocalTestSigner) {
        $Principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
        if (-not $Principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
            throw "-TrustLocalTestSigner requires an elevated PowerShell session"
        }
        if ($Signer.Subject -cne $script:ProbePublisher -or $Signer.Issuer -cne $Signer.Subject) {
            throw "only the self-signed local test certificate for the frozen manifest Publisher may be trusted by this script"
        }
        $EnhancedKeyUsages = @($Signer.EnhancedKeyUsageList | ForEach-Object { [string]$_.ObjectId })
        if ($EnhancedKeyUsages -notcontains "1.3.6.1.5.5.7.3.3") {
            throw "embedded local signer is missing the Code Signing enhanced key usage"
        }
        if (-not (Test-Path $TrustedPeoplePath)) {
            $TemporaryCertificate = Join-Path $env:TEMP "pulsar-probe-signer-$([guid]::NewGuid().ToString('N')).cer"
            try {
                $null = Export-Certificate -Cert $Signer -FilePath $TemporaryCertificate
                $null = Import-Certificate -FilePath $TemporaryCertificate -CertStoreLocation "Cert:\LocalMachine\TrustedPeople"
                $AddedTrust = $true
            } finally {
                Remove-Item -Force $TemporaryCertificate -ErrorAction SilentlyContinue
            }
        }
        $Signature = Get-AuthenticodeSignature -FilePath $PackagePath
    }

    if ($Signature.Status -ne [System.Management.Automation.SignatureStatus]::Valid) {
        throw "package signature status is '$($Signature.Status)'; use -TrustLocalTestSigner only for the generated self-signed test package, or obtain the Store-signed package"
    }
    $ValidatedPackageHash = (Get-FileHash -Path $PackagePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($ValidatedPackageHash -cne $PackageHash) {
        throw "probe package changed between manifest preflight and signature validation"
    }

    Add-AppxPackage -Path $PackagePath
    $InstalledPackages = @(Get-AppxPackage -Name $script:ProbePackageIdentity)
    if ($InstalledPackages.Count -ne 1) {
        throw "Add-AppxPackage registered $($InstalledPackages.Count) matching packages; expected exactly one"
    }
    $Installed = $InstalledPackages[0]
    if ($null -eq $Installed) {
        throw "Add-AppxPackage returned without registering '$script:ProbePackageIdentity'"
    }

    [xml]$InstalledManifest = Get-Content (Join-Path $Installed.InstallLocation "AppxManifest.xml") -Raw
    $Contract = Assert-ProbeManifestContract -Manifest $InstalledManifest
    if ([string]$Installed.Version -cne $ArchiveContract.Version) {
        throw "installed package version differs from the signed archive manifest"
    }
    if ([string]$Installed.Publisher -cne $Contract.Publisher) {
        throw "installed package Publisher differs from the frozen manifest Publisher"
    }
    if ([string]$Installed.Architecture -cne "X64") {
        throw "installed package architecture is '$($Installed.Architecture)', expected X64"
    }
    if ([string]$Installed.PackageFamilyName -cne $Contract.PackageFamilyName) {
        throw "installed package family '$($Installed.PackageFamilyName)' differs from the Publisher-derived family '$($Contract.PackageFamilyName)'"
    }

    $Aumid = "$($Installed.PackageFamilyName)!$($Contract.ApplicationID)"
    $RuntimeRelativeRoot = Get-ProbeRuntimeRelativeRoot
    $RuntimeRoot = Join-Path $env:LOCALAPPDATA $RuntimeRelativeRoot
    if ([string]::IsNullOrWhiteSpace($ReceiptPath)) {
        $ReceiptPath = "$PackagePath.install.json"
    }
    $ReceiptDirectory = Split-Path -Parent $ReceiptPath
    if (-not [string]::IsNullOrWhiteSpace($ReceiptDirectory)) {
        New-Item -ItemType Directory -Force -Path $ReceiptDirectory | Out-Null
    }
    [ordered]@{
        schemaVersion = 2
        verificationBoundary = "hosted-or-local-Windows-install-only; not Win10/Win11 hardware evidence"
        installedAtUTC = [DateTime]::UtcNow.ToString("o")
        packageSha256 = $PackageHash
        packageIdentity = $Contract.PackageIdentity
        publisher = $Contract.Publisher
        version = [string]$Installed.Version
        processorArchitecture = [string]$Installed.Architecture
        packageFullName = [string]$Installed.PackageFullName
        packageFamilyName = [string]$Installed.PackageFamilyName
        applicationUserModelId = $Aumid
        trustLevel = $Contract.TrustLevel
        runtimeBehavior = $Contract.RuntimeBehavior
        capabilities = @($Contract.Capabilities)
        signatureStatusAfterTrust = [string]$Signature.Status
        signerSubject = [string]$Signer.Subject
        signerIssuer = [string]$Signer.Issuer
        signerThumbprint = ([string]$Signer.Thumbprint).ToLowerInvariant()
        signerNotBeforeUTC = $Signer.NotBefore.ToUniversalTime().ToString("o")
        signerNotAfterUTC = $Signer.NotAfter.ToUniversalTime().ToString("o")
        signerTrustAdded = $AddedTrust
        privateSigningMaterialIncluded = $false
        runtimeRootRelativeToLocalAppData = $RuntimeRelativeRoot
        scenarioLogRelativeToLocalAppData = "$RuntimeRelativeRoot\scenarios.jsonl"
        evidenceRelativeToLocalAppData = "$RuntimeRelativeRoot\evidence"
    } | ConvertTo-Json -Depth 4 | Set-Content -Encoding utf8 $ReceiptPath

    if ($Launch) {
        Start-Process explorer.exe -ArgumentList "shell:AppsFolder\$Aumid"
        $LaunchDeadline = [DateTime]::UtcNow.AddSeconds(15)
        $ProbeProcess = $null
        do {
            $ProbeProcess = Get-Process -Name "pulsar-win-probe-amd64" -ErrorAction SilentlyContinue
            if ($null -eq $ProbeProcess) {
                Start-Sleep -Milliseconds 250
            }
        } while ($null -eq $ProbeProcess -and [DateTime]::UtcNow -lt $LaunchDeadline)
        if ($null -eq $ProbeProcess) {
            throw "the package activation command returned but the probe process was not observed"
        }
    }

    Write-Host "Installed package identity: $($Contract.PackageIdentity)"
    Write-Host "Resolved package family: $($Installed.PackageFamilyName)"
    Write-Host "Launch AUMID: $Aumid"
    Write-Host "Scenario log: $(Join-Path $RuntimeRoot 'scenarios.jsonl')"
    Write-Host "Capture artifacts: $(Join-Path $RuntimeRoot 'evidence')"
    Write-Host "Install receipt: $ReceiptPath"

    [pscustomobject]@{
        PackageIdentity = $Contract.PackageIdentity
        PackageFullName = [string]$Installed.PackageFullName
        PackageFamilyName = [string]$Installed.PackageFamilyName
        ApplicationUserModelID = $Aumid
        RuntimeRoot = $RuntimeRoot
        ReceiptPath = $ReceiptPath
        SignerThumbprint = [string]$Signer.Thumbprint
        SignerTrustAdded = $AddedTrust
    }
} catch {
    $CleanupPackages = @($InstalledPackages)
    if ($CleanupPackages.Count -eq 0) {
        $CleanupPackages = @(Get-AppxPackage -Name $script:ProbePackageIdentity -ErrorAction SilentlyContinue)
    }
    if ($CleanupPackages.Count -ne 0) {
        Get-Process -Name "pulsar-win-probe-amd64" -ErrorAction SilentlyContinue |
            Stop-Process -Force -ErrorAction SilentlyContinue
        foreach ($RegisteredPackage in $CleanupPackages) {
            Remove-AppxPackage -Package $RegisteredPackage.PackageFullName -ErrorAction SilentlyContinue
        }
    }
    if ($AddedTrust) {
        Remove-Item -Force $TrustedPeoplePath -ErrorAction SilentlyContinue
    }
    throw
}
