function Invoke-NativeChecked {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][scriptblock]$Command
    )

    & $Command
    $ExitCode = $LASTEXITCODE
    if ($null -eq $ExitCode) {
        throw "$Name did not publish a native process exit code"
    }
    if ($ExitCode -ne 0) {
        throw "$Name failed with exit code $ExitCode"
    }
}
