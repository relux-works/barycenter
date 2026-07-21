$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "register-hidden-interactive-task.ps1") -TaskName "contract-test"

$Contract = Get-HiddenInteractiveTaskActionContract `
    -ObserverScriptPath "C:\ProgramData\Pulsar Evidence\observer.ps1" `
    -ObserverArgumentList @("-EvidencePath", "C:\ProgramData\Pulsar Evidence\result.json")

if ($Contract.ConsoleVisible -or $Contract.WindowStyle -cne "Hidden") {
    throw "observer action contract does not guarantee a hidden PowerShell window"
}
foreach ($Required in @("-NoProfile", "-NonInteractive", "-WindowStyle Hidden", "-ExecutionPolicy Bypass")) {
    if (-not $Contract.Arguments.Contains($Required)) {
        throw "observer action contract is missing '$Required'"
    }
}
if (-not $Contract.Arguments.Contains('"C:\ProgramData\Pulsar Evidence\observer.ps1"')) {
    throw "observer script path was not quoted"
}

$RejectedQuote = $false
try {
    $null = ConvertTo-HiddenTaskArgument -Value 'unsafe"argument'
} catch {
    $RejectedQuote = $true
}
if (-not $RejectedQuote) {
    throw "observer action contract accepted an ambiguous quoted argument"
}

Write-Host "Hidden interactive observer task regressions passed."
