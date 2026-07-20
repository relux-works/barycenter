[CmdletBinding()]
param(
    [ValidateSet("Install", "Status", "Remove")]
    [string]$Mode = "Status",
    [switch]$PhysicalMachineAttested,
    [switch]$ConsoleOperatorAttested,
    [string]$ReceiptPath = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot "native-command.ps1")

$script:BootstrapAuthorizedKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGOCN1+T+fiH1D/TKofJNFqa1tH8BxQzAIXX6N2Ej9GR ivan@relux.works"
$script:BootstrapKeyFingerprint = "SHA256:Ng99XGF2pboYgFVfWJhYI2JRi0PyYsV9UwsJ70NBYd0"
$script:BootstrapKeyComment = "ivan@relux.works"

function Get-BootstrapKeyFingerprint {
    param([Parameter(Mandatory = $true)][string]$AuthorizedKey)

    $Parts = $AuthorizedKey.Trim() -split '\s+'
    if ($Parts.Count -lt 2 -or $Parts[0] -cne "ssh-ed25519") {
        throw "bootstrap key must be one OpenSSH Ed25519 public key"
    }
    try {
        $Blob = [Convert]::FromBase64String($Parts[1])
    } catch {
        throw "bootstrap key payload is not valid base64"
    }
    $Hasher = [Security.Cryptography.SHA256]::Create()
    try {
        $Digest = $Hasher.ComputeHash($Blob)
    } finally {
        $Hasher.Dispose()
    }
    "SHA256:$([Convert]::ToBase64String($Digest).TrimEnd('='))"
}

function Assert-BootstrapKeyContract {
    $Actual = Get-BootstrapKeyFingerprint -AuthorizedKey $script:BootstrapAuthorizedKey
    if ($Actual -cne $script:BootstrapKeyFingerprint) {
        throw "bootstrap public key differs from the reviewed fingerprint"
    }
    if (-not $script:BootstrapAuthorizedKey.EndsWith(" $($script:BootstrapKeyComment)")) {
        throw "bootstrap public key comment differs from the reviewed owner"
    }
}

function Test-IsAdministrator {
    $Identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $Principal = [Security.Principal.WindowsPrincipal]::new($Identity)
    $Principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Get-BootstrapAuthorizedKeysPath {
    Join-Path $env:ProgramData "ssh\administrators_authorized_keys"
}

function Add-BootstrapAuthorizedKey {
    param([Parameter(Mandatory = $true)][string]$Path)

    $Parent = Split-Path -Parent $Path
    if (-not (Test-Path -LiteralPath $Parent -PathType Container)) {
        New-Item -ItemType Directory -Path $Parent -Force | Out-Null
    }
    [string[]]$Existing = @()
    if (Test-Path -LiteralPath $Path -PathType Leaf) {
        $Existing = [string[]][IO.File]::ReadAllLines($Path)
    }
    if (@($Existing | Where-Object { $_.Trim() -ceq $script:BootstrapAuthorizedKey }).Count -ne 0) {
        return $false
    }
    $Prefix = if ($Existing.Count -eq 0) { "" } else { [Environment]::NewLine }
    [IO.File]::AppendAllText(
        $Path,
        "$Prefix$($script:BootstrapAuthorizedKey)$([Environment]::NewLine)",
        [Text.UTF8Encoding]::new($false)
    )
    $true
}

function Remove-BootstrapAuthorizedKey {
    param([Parameter(Mandatory = $true)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return $false
    }
    [string[]]$Existing = [IO.File]::ReadAllLines($Path)
    $Retained = @($Existing | Where-Object { $_.Trim() -cne $script:BootstrapAuthorizedKey })
    if ($Retained.Count -eq $Existing.Count) {
        return $false
    }
    [IO.File]::WriteAllLines($Path, [string[]]$Retained, [Text.UTF8Encoding]::new($false))
    $true
}

function Set-BootstrapAuthorizedKeyACL {
    param([Parameter(Mandatory = $true)][string]$Path)

    Invoke-NativeChecked -Name "authorized-keys ACL" -Command {
        & icacls.exe $Path /inheritance:r /grant "*S-1-5-32-544:F" /grant "*S-1-5-18:F"
    }
}

function Get-BootstrapDeviceProperty {
    param(
        [Parameter(Mandatory = $true)][string]$InstanceID,
        [Parameter(Mandatory = $true)][string]$KeyName
    )

    try {
        $Property = Get-PnpDeviceProperty -InstanceId $InstanceID -KeyName $KeyName -ErrorAction Stop
        [string]$Property.Data
    } catch {
        $null
    }
}

function Get-BootstrapAudioEndpoints {
    $Endpoints = @(Get-PnpDevice -Class AudioEndpoint -PresentOnly -ErrorAction Stop)
    @($Endpoints | Sort-Object FriendlyName, InstanceId | ForEach-Object {
        [ordered]@{
            status = [string]$_.Status
            friendlyName = [string]$_.FriendlyName
            instanceId = [string]$_.InstanceId
            driverProvider = Get-BootstrapDeviceProperty -InstanceID ([string]$_.InstanceId) -KeyName "DEVPKEY_Device_DriverProvider"
            driverVersion = Get-BootstrapDeviceProperty -InstanceID ([string]$_.InstanceId) -KeyName "DEVPKEY_Device_DriverVersion"
            driverDate = Get-BootstrapDeviceProperty -InstanceID ([string]$_.InstanceId) -KeyName "DEVPKEY_Device_DriverDate"
        }
    })
}

function Get-BootstrapPreflight {
    param(
        [Parameter(Mandatory = $true)][bool]$PhysicalAttested,
        [Parameter(Mandatory = $true)][bool]$ConsoleAttested,
        [Parameter(Mandatory = $true)][string]$AuthorizedKeysPath
    )

    $CurrentVersion = Get-ItemProperty "HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion"
    $ComputerSystem = Get-CimInstance Win32_ComputerSystem
    $OperatingSystem = Get-CimInstance Win32_OperatingSystem
    $DeveloperMode = Get-ItemPropertyValue `
        "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock" `
        -Name "AllowDevelopmentWithoutDevLicense" `
        -ErrorAction SilentlyContinue
    if ($null -eq $DeveloperMode) { $DeveloperMode = 0 }
    $SSHD = Get-Service sshd -ErrorAction Stop
    $KnownVirtual = "virtual|vmware|parallels|qemu|xen|hyper-v|cloud|amazon ec2|google compute"
    $LooksVirtual = [string]$ComputerSystem.Manufacturer -match $KnownVirtual -or
        [string]$ComputerSystem.Model -match $KnownVirtual
    if ($LooksVirtual) {
        throw "host reports virtualization; physical evidence bootstrap refuses this machine"
    }

    [ordered]@{
        schemaVersion = 1
        verificationBoundary = "physical-host-access-and-preflight-only; no H00-H17 scenario passed"
        capturedAtUTC = [DateTime]::UtcNow.ToString("o")
        physicalMachineAttested = $PhysicalAttested
        consoleOperatorAttested = $ConsoleAttested
        os = [ordered]@{
            productName = [string]$CurrentVersion.ProductName
            editionId = [string]$CurrentVersion.EditionID
            displayVersion = [string]$CurrentVersion.DisplayVersion
            build = [int]$CurrentVersion.CurrentBuildNumber
            ubr = [int]$CurrentVersion.UBR
            architecture = [string]$OperatingSystem.OSArchitecture
            installType = [string]$CurrentVersion.InstallationType
        }
        hardware = [ordered]@{
            manufacturer = [string]$ComputerSystem.Manufacturer
            model = [string]$ComputerSystem.Model
            hypervisorPresent = [bool]$ComputerSystem.HypervisorPresent
        }
        developerModeValue = [int]$DeveloperMode
        audioEndpoints = @(Get-BootstrapAudioEndpoints)
        wackAvailable = $null -ne (Get-Command appcert.exe -ErrorAction SilentlyContinue)
        ssh = [ordered]@{
            serviceStatus = [string]$SSHD.Status
            authorizedKeysPath = "ProgramData/ssh/administrators_authorized_keys"
            reviewedKeyFingerprint = $script:BootstrapKeyFingerprint
            reviewedKeyPresent = @(Get-Content -LiteralPath $AuthorizedKeysPath -ErrorAction SilentlyContinue |
                Where-Object { $_.Trim() -ceq $script:BootstrapAuthorizedKey }).Count -ne 0
        }
        excludedSensitiveFields = @("hostname", "username", "serial-number", "hardware-uuid", "ip-address")
    }
}

function Write-BootstrapReceipt {
    param(
        [Parameter(Mandatory = $true)]$Value,
        [Parameter(Mandatory = $true)][string]$Path
    )

    $Parent = Split-Path -Parent $Path
    if (-not (Test-Path -LiteralPath $Parent -PathType Container)) {
        New-Item -ItemType Directory -Path $Parent -Force | Out-Null
    }
    $JSON = $Value | ConvertTo-Json -Depth 8
    [IO.File]::WriteAllText($Path, $JSON, [Text.UTF8Encoding]::new($false))
}

function Invoke-BootstrapHardwareHost {
    Assert-BootstrapKeyContract
    if (-not (Test-IsAdministrator)) {
        throw "run this script from an elevated physical-console PowerShell"
    }
    $AuthorizedKeysPath = Get-BootstrapAuthorizedKeysPath
    $ResolvedReceiptPath = if ([string]::IsNullOrWhiteSpace($ReceiptPath)) {
        Join-Path $env:ProgramData "PulsarProbeBootstrap\preflight.json"
    } else {
        [IO.Path]::GetFullPath($ReceiptPath)
    }

    if ($Mode -ceq "Remove") {
        $Removed = Remove-BootstrapAuthorizedKey -Path $AuthorizedKeysPath
        if (Test-Path -LiteralPath $AuthorizedKeysPath -PathType Leaf) {
            Set-BootstrapAuthorizedKeyACL -Path $AuthorizedKeysPath
        }
        [pscustomobject]@{
            Mode = $Mode
            ReviewedKeyRemoved = $Removed
            ReviewedKeyFingerprint = $script:BootstrapKeyFingerprint
            ReceiptPath = $ResolvedReceiptPath
        }
        return
    }

    if ($Mode -ceq "Install") {
        $SSHD = Get-Service sshd -ErrorAction Stop
        if ($SSHD.Status -ne [System.ServiceProcess.ServiceControllerStatus]::Running) {
            throw "OpenSSH sshd must already be running; bootstrap does not change service or firewall policy"
        }
        if (-not $PhysicalMachineAttested -or -not $ConsoleOperatorAttested) {
            throw "Install requires -PhysicalMachineAttested and -ConsoleOperatorAttested"
        }
        $Added = Add-BootstrapAuthorizedKey -Path $AuthorizedKeysPath
        Set-BootstrapAuthorizedKeyACL -Path $AuthorizedKeysPath
    } else {
        $Added = $false
    }

    $Preflight = Get-BootstrapPreflight `
        -PhysicalAttested $PhysicalMachineAttested.IsPresent `
        -ConsoleAttested $ConsoleOperatorAttested.IsPresent `
        -AuthorizedKeysPath $AuthorizedKeysPath
    Write-BootstrapReceipt -Value $Preflight -Path $ResolvedReceiptPath
    Write-Host "SSH_ACCOUNT=$env:USERNAME"
    Write-Host "AUTHORIZED_KEY_FINGERPRINT=$($script:BootstrapKeyFingerprint)"
    Write-Host "PREFLIGHT_RECEIPT=$ResolvedReceiptPath"
    [pscustomobject]@{
        Mode = $Mode
        ReviewedKeyAdded = $Added
        ReviewedKeyPresent = [bool]$Preflight.ssh.reviewedKeyPresent
        ReviewedKeyFingerprint = $script:BootstrapKeyFingerprint
        ReceiptPath = $ResolvedReceiptPath
        Boundary = [string]$Preflight.verificationBoundary
    }
}

if ($MyInvocation.InvocationName -cne ".") {
    Invoke-BootstrapHardwareHost
}
