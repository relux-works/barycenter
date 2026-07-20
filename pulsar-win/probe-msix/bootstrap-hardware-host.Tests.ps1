$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot "bootstrap-hardware-host.ps1")

function Assert-True {
    param([Parameter(Mandatory = $true)][bool]$Condition, [Parameter(Mandatory = $true)][string]$Message)
    if (-not $Condition) { throw $Message }
}

Assert-BootstrapKeyContract
$Fingerprint = Get-BootstrapKeyFingerprint -AuthorizedKey $script:BootstrapAuthorizedKey
Assert-True ($Fingerprint -ceq $script:BootstrapKeyFingerprint) "bootstrap key fingerprint mismatch"

$Root = Join-Path ([IO.Path]::GetTempPath()) "pulsar-bootstrap-$([guid]::NewGuid().ToString('N'))"
$AuthorizedKeys = Join-Path $Root "authorized_keys"
New-Item -ItemType Directory -Path $Root | Out-Null
try {
    $Unrelated = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPNxpBGcArV7mXom1oUGP6y2JmQ4qXv45KfH7rQ7YyxZ unrelated@example"
    [IO.File]::WriteAllText($AuthorizedKeys, "$Unrelated`r`n", [Text.UTF8Encoding]::new($false))
    Assert-True (Add-BootstrapAuthorizedKey -Path $AuthorizedKeys) "first key add did not report a change"
    Assert-True (-not (Add-BootstrapAuthorizedKey -Path $AuthorizedKeys)) "duplicate key add was not idempotent"
    $Lines = @([IO.File]::ReadAllLines($AuthorizedKeys))
    Assert-True (@($Lines | Where-Object { $_ -ceq $Unrelated }).Count -eq 1) "unrelated key was not preserved"
    Assert-True (@($Lines | Where-Object { $_ -ceq $script:BootstrapAuthorizedKey }).Count -eq 1) "reviewed key count mismatch"

    Assert-True (Remove-BootstrapAuthorizedKey -Path $AuthorizedKeys) "first key removal did not report a change"
    Assert-True (-not (Remove-BootstrapAuthorizedKey -Path $AuthorizedKeys)) "duplicate key removal was not idempotent"
    $Retained = @([IO.File]::ReadAllLines($AuthorizedKeys))
    Assert-True (@($Retained | Where-Object { $_ -ceq $Unrelated }).Count -eq 1) "key removal damaged unrelated content"
    Assert-True (@($Retained | Where-Object { $_ -ceq $script:BootstrapAuthorizedKey }).Count -eq 0) "reviewed key remained after removal"
} finally {
    Remove-Item -LiteralPath $Root -Recurse -Force -ErrorAction SilentlyContinue
}

$Source = Get-Content -LiteralPath (Join-Path $PSScriptRoot "bootstrap-hardware-host.ps1") -Raw
foreach ($Forbidden in @("Win32_BIOS", "Win32_ComputerSystemProduct", "SerialNumber", "HardwareUUID")) {
    Assert-True (-not $Source.Contains($Forbidden)) "bootstrap source collects forbidden host identity field '$Forbidden'"
}
foreach ($Required in @(
    "physical-host-access-and-preflight-only; no H00-H17 scenario passed",
    "Install requires -PhysicalMachineAttested and -ConsoleOperatorAttested",
    "bootstrap does not change service or firewall policy",
    "ProgramData/ssh/administrators_authorized_keys",
    "[Security.Cryptography.SHA256]::Create()",
    "[System.ServiceProcess.ServiceControllerStatus]::Running"
)) {
    Assert-True ($Source.Contains($Required)) "bootstrap source lacks fail-closed marker '$Required'"
}

Write-Host "Physical-host bootstrap key, idempotency, privacy and fail-closed regressions passed."
exit 0
