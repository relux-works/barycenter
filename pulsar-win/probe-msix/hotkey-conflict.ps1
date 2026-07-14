[CmdletBinding()]
param(
    [ValidateSet("Probe", "Hold")][string]$Mode = "Probe",
    [ValidateRange(1, 600)][int]$HoldSeconds = 120,
    [string]$ReadyPath = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot "hardware-evidence-contract.ps1")

if ($Mode -ceq "Probe") {
    $Result = Test-ProbeHotkeyAvailability
    $Result | ConvertTo-Json -Compress
    if (-not $Result.Registered) { exit 2 }
    exit 0
}

$Registration = [Pulsar.ProbeHotkeyRegistration]::new()
try {
    if (-not $Registration.Registered) {
        throw "failed to reserve Ctrl+Shift+R for conflict evidence: Win32=$($Registration.Win32Error)"
    }
    if (-not [string]::IsNullOrWhiteSpace($ReadyPath)) {
        $ReadyParent = Split-Path -Parent $ReadyPath
        if (-not [string]::IsNullOrWhiteSpace($ReadyParent)) {
            New-Item -ItemType Directory -Force -Path $ReadyParent | Out-Null
        }
        Write-ProbeEvidenceJSON -Value ([ordered]@{
            schemaVersion = 1
            verificationBoundary = "hotkey-conflict-holder-ready-only; not probe scenario evidence"
            registered = $true
            win32Error = 0
        }) -Path $ReadyPath
    }
    Start-Sleep -Seconds $HoldSeconds
} finally {
    $Registration.Dispose()
}
