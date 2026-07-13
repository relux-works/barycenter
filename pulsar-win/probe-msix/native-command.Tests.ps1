$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "native-command.ps1")

$CaughtExpectedFailure = $false
try {
    Invoke-NativeChecked -Name "intentional native failure" -Command {
        & $env:ComSpec /d /c "exit 23"
    }
} catch {
    if ($_.Exception.Message -eq "intentional native failure failed with exit code 23") {
        $CaughtExpectedFailure = $true
    } else {
        throw
    }
}

if (-not $CaughtExpectedFailure) {
    throw "Invoke-NativeChecked allowed a nonzero native exit code"
}

Write-Host "Native exit-code regression passed."
